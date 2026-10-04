//go:build linux && microvm

package fabric

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// dialTimeout is how long a connection is given to be made: everything here
// is in one kernel, so one that is let through is made at once, and one that
// is dropped is not made at all.
const dialTimeout = time.Second

// helperDialEnv names the address the test binary dials when it is run as a
// helper, as a machine's user.
const helperDialEnv = "WORKLOAD_FABRIC_TEST_DIAL"

// TestNamespace makes the fabric in a network namespace of its own, as vmhost
// does in its holder's, and checks what reaches what. Machines are network
// namespaces joined to the fabric's bridges by veth pairs, which carry
// traffic without a VMM behind them, as a tap would not; the internet is
// another namespace behind the fabric's way out, holding 1.1.1.1 and the
// private and local addresses nothing is to reach. It needs root, and runs
// with -tags microvm, once with each of iptables' backends there is.
func TestNamespace(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("it makes network namespaces, which takes root")
	}

	for _, backend := range []string{"nft", "legacy"} {
		t.Run(backend, func(t *testing.T) {
			save, err := exec.LookPath("iptables-" + backend + "-save")
			if err != nil {
				t.Skipf("iptables' %s backend is not here", backend)
			}

			restore, err := exec.LookPath("iptables-" + backend + "-restore")
			require.NoError(t, err)

			scenario(t, iptables{saveBinary: save, restoreBinary: restore}, "iptables-"+backend)
		})
	}
}

func scenario(t *testing.T, firewall iptables, iptablesBinary string) {
	const (
		first    = 1_000_000_000
		isolated = "workload-isolated"
		stack    = "workload-stack-shop"
	)

	helper := copyExecutable(t)

	fabricNS := newNamespace(t)
	internetNS := newNamespace(t)

	// the way out: the fabric's namespace routes everything it does not hold
	// through 198.51.100.1, which is the internet's.
	veth(t, fabricNS, "up0", internetNS, "up1")
	configure(t, fabricNS, "up0", "198.51.100.2/24", nil, "198.51.100.1")
	configure(t, internetNS, "up1", "198.51.100.1/24", nil, "")

	internet := handleAt(t, internetNS)
	require.NoError(t, internet.LinkAdd(&netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "inet0"}}))
	inet0, err := internet.LinkByName("inet0")
	require.NoError(t, err)

	for _, address := range []string{"1.1.1.1/32", "169.254.169.254/32", "10.0.0.1/32", "172.18.0.1/32", "192.168.1.1/32", "100.64.0.1/32"} {
		parsed, err := netlink.ParseAddr(address)
		require.NoError(t, err)
		require.NoError(t, internet.AddrAdd(inet0, parsed))
	}
	require.NoError(t, internet.LinkSetUp(inet0))

	seen := listen(t, internetNS, "0.0.0.0:80")
	listen(t, internetNS, "0.0.0.0:3333")

	// vmhost's own services, in the fabric's namespace.
	listen(t, fabricNS, "0.0.0.0:9090")

	recorded := &recording{tables: firewall}
	config := Config{Dir: t.TempDir(), Pool: cidr(t, "10.250.0.0/16"), BlockedPorts: []uint16{3333}, FirstUID: first, UIDs: 65536}

	var fabric *Fabric
	in(t, fabricNS, func() {
		// a namespace that forwards nothing — a new one takes the host's
		// setting, so this one is told to — is refused rather than having
		// forwarding turned on: wherever the fabric runs, the host's
		// namespace included, it changes no setting of the namespace's.
		require.NoError(t, os.WriteFile(forwardingPath, []byte("0"), 0o644))

		_, err = newFabric(config, slog.New(slog.DiscardHandler), recorded)
		require.ErrorIs(t, err, vm.ErrUnavailable)
		assert.Zero(t, recorded.restores, "nothing is written to a namespace the fabric refused")

		forwarding, err := os.ReadFile(forwardingPath)
		require.NoError(t, err)
		assert.Equal(t, "0", strings.TrimSpace(string(forwarding)), "forwarding is left as it was")

		// what the holder container's sysctls do, for its namespace alone.
		require.NoError(t, os.WriteFile(forwardingPath, []byte("1"), 0o644))

		fabric, err = newFabric(config, slog.New(slog.DiscardHandler), recorded)
		require.NoError(t, err)
	})

	ctx := context.Background()

	var public, internal vm.Network
	in(t, fabricNS, func() {
		public, err = fabric.EnsureNetwork(ctx, vm.PublicNetwork, true)
		require.NoError(t, err)

		internal, err = fabric.EnsureNetwork(ctx, isolated, false)
		require.NoError(t, err)

		again, err := fabric.EnsureNetwork(ctx, isolated, false)
		require.NoError(t, err)
		assert.Equal(t, internal, again, "a network that is there is the one asked for")

		_, err = fabric.EnsureNetwork(ctx, vm.PublicNetwork, false)
		assert.ErrorIs(t, err, vm.ErrConflict, "a network does not start or stop routing out")
	})

	assert.Equal(t, vm.Network{Name: vm.PublicNetwork, Subnet: "10.250.0.0/24", Gateway: "10.250.0.1", Masquerade: true}, public)
	assert.Equal(t, vm.Network{Name: isolated, Subnet: "10.250.1.0/24", Gateway: "10.250.1.1"}, internal)

	// four machines: two on the isolated network, two on the public one.
	machines := []struct {
		id         string
		uid        int
		attachment vm.Attachment
		ns         netns.NsHandle
		iface      vm.Interface
	}{
		{id: "0000000000000001", uid: first + 1, attachment: vm.Attachment{Network: isolated, Aliases: []string{"one"}}},
		{id: "0000000000000002", uid: first + 2, attachment: vm.Attachment{Network: isolated}},
		{id: "0000000000000003", uid: first + 3, attachment: vm.Attachment{Network: vm.PublicNetwork, Gateway: true}},
		{id: "0000000000000004", uid: first + 4, attachment: vm.Attachment{Network: vm.PublicNetwork, Gateway: true}},
	}

	for i := range machines {
		machine := &machines[i]

		in(t, fabricNS, func() {
			interfaces, err := fabric.Plug(ctx, machine.id, machine.uid, []vm.Attachment{machine.attachment})
			require.NoError(t, err)
			require.Len(t, interfaces, 1)

			machine.iface = interfaces[0]
		})
	}

	t.Run("a machine is given a tap of its user's on each network, and an address", func(t *testing.T) {
		assert.Equal(t, vm.Interface{Network: isolated, Device: tapName(machines[0].id, 0), MAC: "06:00:0a:fa:01:02", Address: "10.250.1.2/24", Aliases: []string{"one"}}, machines[0].iface)
		assert.Equal(t, "10.250.1.3/24", machines[1].iface.Address)
		assert.Equal(t, vm.Interface{Network: vm.PublicNetwork, Device: tapName(machines[2].id, 0), MAC: "06:00:0a:fa:00:02", Address: "10.250.0.2/24", Gateway: "10.250.0.1"}, machines[2].iface)

		in(t, fabricNS, func() {
			for _, machine := range machines {
				link, err := netlink.LinkByName(machine.iface.Device)
				require.NoError(t, err)

				tap, ok := link.(*netlink.Tuntap)
				require.True(t, ok, "%s is a tap", machine.iface.Device)
				assert.Equal(t, uint32(machine.uid), tap.Owner, "the tap is its machine's user's to open")
				assert.Equal(t, uint32(machine.uid), tap.Group)
				assert.Equal(t, tapAlias(machine.id, machine.attachment.Network), tap.Attrs().Alias)

				bridge, err := netlink.LinkByName(bridgeName(machine.attachment.Network))
				require.NoError(t, err)
				assert.Equal(t, bridge.Attrs().Index, tap.Attrs().MasterIndex)

				protinfo, err := netlink.LinkGetProtinfo(tap)
				require.NoError(t, err)
				assert.Equal(t, machine.attachment.Network == vm.PublicNetwork, protinfo.Isolated, "only the public network's ports are kept from each other")
			}

			bridge, err := netlink.LinkByName(bridgeName(vm.PublicNetwork))
			require.NoError(t, err)
			assert.Equal(t, "06:00:0a:fa:00:01", bridge.Attrs().HardwareAddr.String(), "a bridge's MAC is its own, and does not move with its ports")
		})
	})

	t.Run("a network a machine joins that is not there is made", func(t *testing.T) {
		in(t, fabricNS, func() {
			interfaces, err := fabric.Plug(ctx, "0000000000000005", first+5, []vm.Attachment{{Network: stack}})
			require.NoError(t, err)
			assert.Equal(t, "10.250.2.2/24", interfaces[0].Address)

			networks, err := fabric.Networks(ctx)
			require.NoError(t, err)
			assert.Equal(t, []vm.Network{
				{Name: vm.PublicNetwork, Subnet: "10.250.0.0/24", Gateway: "10.250.0.1", Masquerade: true},
				{Name: isolated, Subnet: "10.250.1.0/24", Gateway: "10.250.1.1"},
				{Name: stack, Subnet: "10.250.2.0/24", Gateway: "10.250.2.1"},
			}, networks)
		})
	})

	// the machines themselves: a veth each, plugged in as its tap is, and on
	// the machine's side what the guest would set up from its interface.
	// Every machine routes everything through its network's gateway, as one
	// that tried to reach out would.
	for i := range machines {
		machine := &machines[i]
		machine.ns = newNamespace(t)

		hostSide, guestSide := "wkv"+machine.id[15:], "wkg"+machine.id[15:]
		veth(t, fabricNS, hostSide, machine.ns, guestSide)

		in(t, fabricNS, func() {
			link, err := netlink.LinkByName(hostSide)
			require.NoError(t, err)

			networks, err := fabric.networks()
			require.NoError(t, err)

			n, found := find(networks, machine.attachment.Network)
			require.True(t, found)
			require.NoError(t, connect(link, n))
		})

		mac, err := net.ParseMAC(machine.iface.MAC)
		require.NoError(t, err)

		gw := public.Gateway
		if machine.attachment.Network == isolated {
			gw = internal.Gateway
		}

		configure(t, machine.ns, guestSide, machine.iface.Address, mac, gw)
	}

	listen(t, machines[1].ns, "0.0.0.0:8080")
	listen(t, machines[3].ns, "0.0.0.0:8080")

	addressOf := func(i int) string {
		address, _, _ := strings.Cut(machines[i].iface.Address, "/")

		return address
	}

	t.Run("machines on one network reach each other", func(t *testing.T) {
		assert.NoError(t, dial(t, machines[0].ns, addressOf(1)+":8080"))
	})

	t.Run("an isolated network does not reach the internet", func(t *testing.T) {
		assert.Error(t, dial(t, machines[0].ns, "1.1.1.1:80"))
	})

	t.Run("a public network reaches the internet, masqueraded as the namespace", func(t *testing.T) {
		require.NoError(t, dial(t, machines[2].ns, "1.1.1.1:80"))

		select {
		case remote := <-seen:
			assert.True(t, strings.HasPrefix(remote, "198.51.100.2:"), remote)
		case <-time.After(time.Second):
			t.Fatal("the connection never arrived")
		}
	})

	t.Run("a public network reaches nothing private or local, nor a blocked port", func(t *testing.T) {
		for _, address := range []string{"1.1.1.1:3333", "169.254.169.254:80", "10.0.0.1:80", "172.18.0.1:80", "192.168.1.1:80", "100.64.0.1:80"} {
			assert.Error(t, dial(t, machines[2].ns, address), address)
		}
	})

	t.Run("machines on the public network do not reach each other", func(t *testing.T) {
		assert.Error(t, dial(t, machines[2].ns, addressOf(3)+":8080"))
	})

	t.Run("machines do not reach other networks' machines", func(t *testing.T) {
		assert.Error(t, dial(t, machines[0].ns, addressOf(3)+":8080"))
	})

	t.Run("nothing reaches the fabric's own addresses", func(t *testing.T) {
		assert.Error(t, dial(t, machines[0].ns, internal.Gateway+":9090"))
		assert.Error(t, dial(t, machines[2].ns, public.Gateway+":9090"))
		assert.Error(t, dial(t, machines[2].ns, "198.51.100.2:9090"))
	})

	t.Run("nothing a machine's user sends of its own goes anywhere", func(t *testing.T) {
		assert.NoError(t, dialAs(t, fabricNS, helper, 0, "1.1.1.1:80"), "the namespace itself reaches out")
		assert.Error(t, dialAs(t, fabricNS, helper, first+3, "1.1.1.1:80"))
	})

	t.Run("a firewall as it should be is not written again", func(t *testing.T) {
		writes := recorded.restores

		in(t, fabricNS, func() {
			require.NoError(t, fabric.Repair(ctx))
		})

		assert.Equal(t, writes, recorded.restores)
	})

	t.Run("the firewall is repaired after something flushed it", func(t *testing.T) {
		in(t, fabricNS, func() {
			output, err := exec.Command(iptablesBinary, "-F", forwardChain).CombinedOutput()
			require.NoError(t, err, string(output))
		})

		assert.NoError(t, dial(t, machines[2].ns, "169.254.169.254:80"), "flushed, the firewall lets through what it should not")

		writes := recorded.restores

		in(t, fabricNS, func() {
			require.NoError(t, fabric.Repair(ctx))
		})

		assert.Equal(t, writes+1, recorded.restores)
		assert.Error(t, dial(t, machines[2].ns, "169.254.169.254:80"))
		assert.NoError(t, dial(t, machines[2].ns, "1.1.1.1:80"))
		assert.NoError(t, dial(t, machines[0].ns, addressOf(1)+":8080"))
	})

	t.Run("a vmhost started again finds the networks, the firewall and the addresses as they were", func(t *testing.T) {
		again := &recording{tables: firewall}

		in(t, fabricNS, func() {
			restarted, err := newFabric(config, slog.New(slog.DiscardHandler), again)
			require.NoError(t, err)

			assert.Zero(t, again.restores, "nothing is written, so nothing running notices")

			networks, err := restarted.Networks(ctx)
			require.NoError(t, err)
			assert.Len(t, networks, 3)

			interfaces, err := restarted.Plug(ctx, "0000000000000006", first+6, []vm.Attachment{{Network: isolated}})
			require.NoError(t, err)
			assert.Equal(t, "10.250.1.4/24", interfaces[0].Address, "an address held before is not handed out again")

			fabric = restarted
		})

		assert.NoError(t, dial(t, machines[0].ns, addressOf(1)+":8080"))
	})

	t.Run("a machine unplugged gives back its taps and its address", func(t *testing.T) {
		in(t, fabricNS, func() {
			require.NoError(t, fabric.Unplug(ctx, machines[3].id))

			_, err := netlink.LinkByName(machines[3].iface.Device)
			assert.ErrorAs(t, err, &netlink.LinkNotFoundError{})

			interfaces, err := fabric.Plug(ctx, "0000000000000007", first+7, []vm.Attachment{{Network: vm.PublicNetwork, Gateway: true}})
			require.NoError(t, err)
			assert.Equal(t, machines[3].iface.Address, interfaces[0].Address)

			require.NoError(t, fabric.Unplug(ctx, "0000000000000007"))
		})
	})

	t.Run("what machines that are gone held is given back", func(t *testing.T) {
		in(t, fabricNS, func() {
			require.NoError(t, fabric.Retain(ctx, []string{machines[0].id, machines[2].id, "0000000000000005", "0000000000000006"}))

			_, err := netlink.LinkByName(machines[1].iface.Device)
			assert.ErrorAs(t, err, &netlink.LinkNotFoundError{}, "a gone machine's tap is taken away")

			_, err = netlink.LinkByName(machines[0].iface.Device)
			assert.NoError(t, err, "a kept machine's tap stays")

			assert.False(t, holds(fabric.leases.networks[isolated].Holders, machines[1].id), "and its address given back")
			assert.True(t, holds(fabric.leases.networks[isolated].Holders, machines[0].id))
		})
	})

	t.Run("a network is taken away once nothing is plugged into it, and its rules with it", func(t *testing.T) {
		in(t, fabricNS, func() {
			assert.ErrorIs(t, fabric.RemoveNetwork(ctx, isolated), vm.ErrNetworkInUse)

			require.NoError(t, fabric.Unplug(ctx, machines[0].id))
			require.NoError(t, fabric.Unplug(ctx, "0000000000000006"))

			assert.ErrorIs(t, fabric.RemoveNetwork(ctx, isolated), vm.ErrNetworkInUse, "the machines' veths are still plugged in")

			for _, side := range []string{"wkv1", "wkv2"} {
				link, err := netlink.LinkByName(side)
				require.NoError(t, err)
				require.NoError(t, netlink.LinkDel(link))
			}

			require.NoError(t, fabric.RemoveNetwork(ctx, isolated))
			require.NoError(t, fabric.RemoveNetwork(ctx, isolated), "one that is not there is the outcome asked for")

			_, err := netlink.LinkByName(bridgeName(isolated))
			assert.ErrorAs(t, err, &netlink.LinkNotFoundError{})

			saved, err := firewall.save(ctx)
			require.NoError(t, err)
			assert.NotContains(t, saved, "10.250.1.0/24")
			assert.Contains(t, saved, "10.250.0.0/24")
		})
	})
}

// TestHelperDial is the test binary dialing as somebody else: it is only run
// by scenario, as a machine's user would.
func TestHelperDial(t *testing.T) {
	address := os.Getenv(helperDialEnv)
	if address == "" {
		t.Skip("only run as a helper")
	}

	conn, err := net.DialTimeout("tcp", address, dialTimeout)
	if err != nil {
		os.Exit(3)
	}

	conn.Close()
	os.Exit(0)
}

// recording counts what is written to a firewall.
type recording struct {
	tables
	restores int
}

func (r *recording) restore(ctx context.Context, input string) error {
	r.restores++

	return r.tables.restore(ctx, input)
}

// in runs do with the calling goroutine in a network namespace.
func in(t *testing.T, ns netns.NsHandle, do func()) {
	t.Helper()

	runtime.LockOSThread()

	origin, err := netns.Get()
	require.NoError(t, err)

	defer func() {
		// a thread that cannot be put back stays locked, and goes with its
		// goroutine.
		if err := netns.Set(origin); err != nil {
			t.Errorf("the thread could not be put back in its namespace: %v", err)

			return
		}

		origin.Close()
		runtime.UnlockOSThread()
	}()

	require.NoError(t, netns.Set(ns))

	do()
}

// newNamespace is a network namespace of the test's own, with its loopback up.
func newNamespace(t *testing.T) netns.NsHandle {
	t.Helper()

	var created netns.NsHandle

	func() {
		runtime.LockOSThread()

		origin, err := netns.Get()
		require.NoError(t, err)

		created, err = netns.New()
		require.NoError(t, err)

		if err := netns.Set(origin); err != nil {
			t.Fatalf("the thread could not be put back in its namespace: %v", err)
		}

		origin.Close()
		runtime.UnlockOSThread()
	}()

	t.Cleanup(func() { created.Close() })

	handle := handleAt(t, created)

	lo, err := handle.LinkByName("lo")
	require.NoError(t, err)
	require.NoError(t, handle.LinkSetUp(lo))

	return created
}

func handleAt(t *testing.T, ns netns.NsHandle) *netlink.Handle {
	t.Helper()

	handle, err := netlink.NewHandleAt(ns)
	require.NoError(t, err)
	t.Cleanup(handle.Close)

	return handle
}

// veth joins two namespaces with a veth pair.
func veth(t *testing.T, from netns.NsHandle, name string, to netns.NsHandle, peer string) {
	t.Helper()

	handle := handleAt(t, from)
	require.NoError(t, handle.LinkAdd(&netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: name}, PeerName: peer}))

	moved, err := handle.LinkByName(peer)
	require.NoError(t, err)
	require.NoError(t, handle.LinkSetNsFd(moved, int(to)))
}

// configure gives a device its address, and its MAC if one is given, sets it
// up, and routes everything through gateway if one is given.
func configure(t *testing.T, ns netns.NsHandle, device string, address string, mac net.HardwareAddr, gateway string) {
	t.Helper()

	handle := handleAt(t, ns)

	link, err := handle.LinkByName(device)
	require.NoError(t, err)

	if mac != nil {
		require.NoError(t, handle.LinkSetHardwareAddr(link, mac))
	}

	parsed, err := netlink.ParseAddr(address)
	require.NoError(t, err)
	require.NoError(t, handle.AddrAdd(link, parsed))
	require.NoError(t, handle.LinkSetUp(link))

	if gateway != "" {
		require.NoError(t, handle.RouteAdd(&netlink.Route{LinkIndex: link.Attrs().Index, Gw: net.ParseIP(gateway)}))
	}
}

// listen serves an address in a namespace, and says where every connection
// came from.
func listen(t *testing.T, ns netns.NsHandle, address string) <-chan string {
	t.Helper()

	var listener net.Listener

	in(t, ns, func() {
		var err error

		listener, err = net.Listen("tcp", address)
		require.NoError(t, err)
	})

	t.Cleanup(func() { listener.Close() })

	seen := make(chan string, 64)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			select {
			case seen <- conn.RemoteAddr().String():
			default:
			}

			conn.Close()
		}
	}()

	return seen
}

// dial connects to an address from a namespace.
func dial(t *testing.T, ns netns.NsHandle, address string) error {
	t.Helper()

	var err error

	in(t, ns, func() {
		var conn net.Conn

		conn, err = net.DialTimeout("tcp", address, dialTimeout)
		if err == nil {
			conn.Close()
		}
	})

	return err
}

// dialAs connects to an address from a namespace, as uid, through the test
// binary run as a helper: a process of that user's, as a machine's VMM is.
func dialAs(t *testing.T, ns netns.NsHandle, helper string, uid int, address string) error {
	t.Helper()

	var err error

	in(t, ns, func() {
		command := exec.Command(helper, "-test.run=^TestHelperDial$", "-test.count=1")
		command.Dir = filepath.Dir(helper)
		command.Env = append(os.Environ(), helperDialEnv+"="+address)
		command.Stdout = io.Discard
		command.Stderr = io.Discard

		if uid != 0 {
			command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(uid)}}
		}

		err = command.Run()
	})

	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("the helper could not be run: %v", err)
	}

	return err
}

// copyExecutable copies the test binary where any user may run it.
func copyExecutable(t *testing.T) string {
	t.Helper()

	self, err := os.Executable()
	require.NoError(t, err)

	content, err := os.ReadFile(self)
	require.NoError(t, err)

	dir, err := os.MkdirTemp("", "fabric-helper-")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	require.NoError(t, os.Chmod(dir, 0o755))

	helper := filepath.Join(dir, "fabric.test")
	require.NoError(t, os.WriteFile(helper, content, 0o755))

	return helper
}

// holds reports whether a machine holds any of a network's addresses.
func holds(holders map[string]string, id string) bool {
	for _, holder := range holders {
		if holder == id {
			return true
		}
	}

	return false
}
