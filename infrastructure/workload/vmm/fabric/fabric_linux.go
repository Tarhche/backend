//go:build linux

package fabric

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// setupTimeout bounds reading and writing the firewall when the fabric is
// made, which has nothing else to bound it.
const setupTimeout = 30 * time.Second

// New prepares machines' networks in the namespace vmhost runs in: forwarding
// on, and the firewall as the networks already there say it should be. A
// vmhost that starts again finds the firewall as it left it, and changes
// nothing, so machines running across the restart never notice.
func New(config Config, logger *slog.Logger) (*Fabric, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	firewall, err := iptablesTables()
	if err != nil {
		return nil, err
	}

	return newFabric(config, logger, firewall)
}

// newFabric is New with the firewall given.
func newFabric(config Config, logger *slog.Logger, firewall tables) (*Fabric, error) {
	book, err := openLeases(config.Dir)
	if err != nil {
		return nil, err
	}

	repairs, err := otel.Meter("github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/fabric").Int64Counter(
		"workload.vmhost.firewall.repairs",
		metric.WithDescription("Times the firewall of the machines' networks was found changed by something else, and put back."),
		metric.WithUnit("{repair}"),
	)
	if err != nil {
		return nil, err
	}

	f := &Fabric{config: config, logger: logger, tables: firewall, leases: book, repairs: repairs}

	if err := forwarding(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	networks, err := f.networks()
	if err != nil {
		return nil, err
	}

	reasons, err := f.apply(ctx, networks)
	if err != nil {
		return nil, err
	}

	if len(reasons) > 0 {
		logger.Info("firewall of the machines' networks written", "networks", len(networks), "because", strings.Join(reasons, "; "))
	}

	return f, nil
}

func (f *Fabric) EnsureNetwork(ctx context.Context, name string, masquerade bool) (vm.Network, error) {
	if !vm.IsNetworkName(name) {
		return vm.Network{}, fmt.Errorf("%w: %q is not a network", vm.ErrInvalid, name)
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	networks, err := f.networks()
	if err != nil {
		return vm.Network{}, err
	}

	if existing, found := find(networks, name); found {
		if existing.masquerade != masquerade {
			return vm.Network{}, fmt.Errorf("%w: %s is a network already, and whether it routes out is not something it can start or stop doing", vm.ErrConflict, name)
		}

		return existing.view(), nil
	}

	made, err := f.make(ctx, networks, name, masquerade)
	if err != nil {
		return vm.Network{}, err
	}

	return made.view(), nil
}

// make makes a network that is not there, and the firewall for it. The lock
// is held.
func (f *Fabric) make(ctx context.Context, networks []network, name string, masquerade bool) (network, error) {
	taken := make([]*net.IPNet, 0, len(networks))
	for _, n := range networks {
		taken = append(taken, n.subnet)
	}

	subnet, err := allocate(f.config.Pool, taken, f.leases.subnetOf(name))
	if err != nil {
		return network{}, err
	}

	made := network{name: name, subnet: subnet, masquerade: masquerade}

	bridge, err := makeBridge(made)
	if err != nil {
		return network{}, err
	}

	// a network the firewall does not know of is no network: what it
	// carries would be dropped as anything else of the pool's is, and it is
	// better not there at all.
	if _, err := f.apply(ctx, append(slices.Clone(networks), made)); err != nil {
		return network{}, errors.Join(err, netlink.LinkDel(bridge))
	}

	f.leases.on(name, subnet)
	if err := f.leases.save(); err != nil {
		f.logger.Warn("failed to write down the subnet a network was made on", "network", name, "error", err)
	}

	f.logger.Info("network made", "network", name, "subnet", subnet.String(), "masquerade", masquerade, "bridge", bridge.Attrs().Name)

	return made, nil
}

// makeBridge makes the bridge a network is, carrying what it is for in its
// alias, with the fabric's address on it and a MAC of its own that never
// changes under its machines.
func makeBridge(n network) (netlink.Link, error) {
	name := bridgeName(n.name)

	// a bridge of that name that is not a network — left by a vmhost that
	// went away before it said what it was for — is made again.
	if stale, err := netlink.LinkByName(name); err == nil {
		if err := netlink.LinkDel(stale); err != nil {
			return nil, err
		}
	}

	if err := netlink.LinkAdd(&netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: name, HardwareAddr: macOf(gateway(n.subnet))}}); err != nil {
		return nil, fmt.Errorf("failed to make the bridge for %s: %w", n.name, err)
	}

	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, err
	}

	fail := func(err error) (netlink.Link, error) {
		return nil, errors.Join(err, netlink.LinkDel(link))
	}

	if err := netlink.LinkSetAlias(link, alias(n.name, n.masquerade)); err != nil {
		return fail(err)
	}

	address := &netlink.Addr{IPNet: &net.IPNet{IP: gateway(n.subnet), Mask: n.subnet.Mask}}
	if err := netlink.AddrAdd(link, address); err != nil {
		return fail(err)
	}

	disableIPv6(name)

	if err := netlink.LinkSetUp(link); err != nil {
		return fail(err)
	}

	return link, nil
}

func (f *Fabric) RemoveNetwork(ctx context.Context, name string) error {
	if !vm.IsNetworkName(name) {
		return fmt.Errorf("%w: %q is not a network", vm.ErrInvalid, name)
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	networks, err := f.networks()
	if err != nil {
		return err
	}

	if _, found := find(networks, name); !found {
		f.leases.forget(name)

		return f.leases.save()
	}

	bridge, err := netlink.LinkByName(bridgeName(name))
	if err != nil {
		return err
	}

	plugged, err := ports(bridge)
	if err != nil {
		return err
	}

	if len(plugged) > 0 || f.leases.inUse(name) {
		return fmt.Errorf("%w: %s", vm.ErrNetworkInUse, name)
	}

	if err := netlink.LinkDel(bridge); err != nil {
		return err
	}

	f.leases.forget(name)
	if err := f.leases.save(); err != nil {
		return err
	}

	remaining := slices.DeleteFunc(networks, func(n network) bool { return n.name == name })
	if _, err := f.apply(ctx, remaining); err != nil {
		return err
	}

	f.logger.Info("network removed", "network", name)

	return nil
}

func (f *Fabric) Networks(ctx context.Context) ([]vm.Network, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	networks, err := f.networks()
	if err != nil {
		return nil, err
	}

	views := make([]vm.Network, 0, len(networks))
	for _, n := range networks {
		views = append(views, n.view())
	}

	return views, nil
}

// Plug gives a machine a tap on each network it joins, and an address on each.
// A network that is not there is made, routing out if it is the public one: a
// host that restarted lost its bridges, and a machine started again by its
// restart policy finds them made again rather than missing. Nothing of a plug
// that fails is left: not its taps, not its addresses.
func (f *Fabric) Plug(ctx context.Context, id string, uid int, attachments []vm.Attachment) ([]vm.Interface, error) {
	if err := validatePlug(id, attachments); err != nil {
		return nil, err
	}

	owner, err := f.config.owner(uid, os.Geteuid())
	if err != nil {
		return nil, err
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	interfaces, err := f.plug(ctx, id, owner, attachments)
	if err != nil {
		return nil, errors.Join(err, f.unplug(id))
	}

	f.logger.Debug("machine plugged in", "machine", id, "interfaces", len(interfaces))

	return interfaces, nil
}

// plug is Plug with the lock held.
func (f *Fabric) plug(ctx context.Context, id string, owner int, attachments []vm.Attachment) ([]vm.Interface, error) {
	// taps left by an earlier start of the same machine are taken away
	// first, so what is plugged in is what is asked for now. Addresses it
	// held on the networks it joins again are kept.
	if err := deleteTaps(func(device string) bool { return strings.HasPrefix(device, tapPrefixOf(id)) }); err != nil {
		return nil, err
	}

	joined := make([]string, 0, len(attachments))
	interfaces := make([]vm.Interface, 0, len(attachments))

	for i, attachment := range attachments {
		n, err := f.join(ctx, attachment.Network)
		if err != nil {
			return nil, err
		}

		address, err := f.leases.hand(n.name, n.subnet, id)
		if err != nil {
			return nil, err
		}

		device := tapName(id, i)
		if err := makeTap(device, owner, id, n); err != nil {
			return nil, err
		}

		ones, _ := n.subnet.Mask.Size()

		plugged := vm.Interface{
			Network: n.name,
			Device:  device,
			MAC:     macOf(address).String(),
			Address: fmt.Sprintf("%s/%d", address, ones),
			Aliases: slices.Clone(attachment.Aliases),
		}

		if attachment.Gateway {
			plugged.Gateway = gateway(n.subnet).String()
		}

		joined = append(joined, n.name)
		interfaces = append(interfaces, plugged)
	}

	f.leases.keepOnly(id, joined)

	if err := f.leases.save(); err != nil {
		return nil, err
	}

	return interfaces, nil
}

// join is the network a machine joins, made if it is not there. The lock is
// held.
func (f *Fabric) join(ctx context.Context, name string) (network, error) {
	networks, err := f.networks()
	if err != nil {
		return network{}, err
	}

	if existing, found := find(networks, name); found {
		return existing, nil
	}

	return f.make(ctx, networks, name, name == vm.PublicNetwork)
}

// makeTap makes one of a machine's taps, plugged into its network's bridge.
// The tap is its machine's user's alone to open, which is what lets the
// machine's VMM open it without any privilege at all.
func makeTap(device string, owner int, id string, n network) error {
	tap := &netlink.Tuntap{
		LinkAttrs: netlink.LinkAttrs{Name: device},
		Mode:      netlink.TUNTAP_MODE_TAP,
		Flags:     netlink.TUNTAP_NO_PI,
		Owner:     uint32(owner),
		Group:     uint32(owner),
	}

	if err := netlink.LinkAdd(tap); err != nil {
		return fmt.Errorf("failed to make a tap on %s: %w", n.name, err)
	}

	link, err := netlink.LinkByName(device)
	if err != nil {
		return err
	}

	if err := netlink.LinkSetAlias(link, tapAlias(id, n.name)); err != nil {
		return err
	}

	return connect(link, n)
}

// connect plugs a device into a network's bridge. Machines on a public
// network do not reach each other: the network is there to route out, and
// gives them nothing else in common, so its ports are isolated from each
// other.
func connect(link netlink.Link, n network) error {
	bridge, err := netlink.LinkByName(bridgeName(n.name))
	if err != nil {
		return fmt.Errorf("the bridge of %s is not there: %w", n.name, err)
	}

	if err := netlink.LinkSetMaster(link, bridge); err != nil {
		return err
	}

	if n.masquerade {
		if err := netlink.LinkSetIsolated(link, true); err != nil {
			return err
		}
	}

	disableIPv6(link.Attrs().Name)

	return netlink.LinkSetUp(link)
}

func (f *Fabric) Unplug(ctx context.Context, id string) error {
	if !vm.IsID(id) {
		return fmt.Errorf("%w: %q is not a machine", vm.ErrInvalid, id)
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	return f.unplug(id)
}

// unplug takes a machine's taps away and gives back its addresses. The lock
// is held.
func (f *Fabric) unplug(id string) error {
	deleted := deleteTaps(func(device string) bool { return strings.HasPrefix(device, tapPrefixOf(id)) })

	f.leases.release(id)

	return errors.Join(deleted, f.leases.save())
}

// Retain gives back the taps and addresses of every machine not named. Every
// machine vmhost is starting has to be named too: its taps are made before it
// runs.
func (f *Fabric) Retain(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if !vm.IsID(id) {
			return fmt.Errorf("%w: %q is not a machine", vm.ErrInvalid, id)
		}
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	prefixes := make([]string, 0, len(ids))
	for _, id := range ids {
		prefixes = append(prefixes, tapPrefixOf(id))
	}

	var taps []string

	deleted := deleteTaps(func(device string) bool {
		if !isTap(device) || slices.Contains(prefixes, device[:len(device)-1]) {
			return false
		}

		taps = append(taps, device)

		return true
	})

	released := f.leases.retain(func(id string) bool { return slices.Contains(ids, id) })

	if len(taps) > 0 || len(released) > 0 {
		f.logger.Info("what machines that are gone held is given back", "taps", taps, "machines", released)
	}

	return errors.Join(deleted, f.leases.save())
}

// Repair puts the firewall back as the networks say it should be. Finding it
// changed means something else on the host changed it — flushed it, or
// restarted what keeps a firewall there — which is worth knowing: it is
// logged, and counted.
func (f *Fabric) Repair(ctx context.Context) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	networks, err := f.networks()
	if err != nil {
		return err
	}

	reasons, err := f.apply(ctx, networks)
	if err != nil {
		return err
	}

	if len(reasons) > 0 {
		f.repairs.Add(ctx, 1)
		f.logger.Warn("the firewall of the machines' networks was changed by something else, and is put back", "because", strings.Join(reasons, "; "))
	}

	return nil
}

// networks is every network the kernel holds, read off the bridges
// themselves, by name.
func (f *Fabric) networks() ([]network, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	var networks []network

	for _, link := range links {
		if link.Type() != "bridge" || !strings.HasPrefix(link.Attrs().Name, bridgePrefix) {
			continue
		}

		name, masquerade, ok := parseAlias(link.Attrs().Alias)
		if !ok || bridgeName(name) != link.Attrs().Name {
			continue
		}

		addresses, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return nil, err
		}

		for _, address := range addresses {
			subnet := &net.IPNet{IP: address.IP.Mask(address.Mask).To4(), Mask: address.Mask}

			if address.IP.Equal(gateway(subnet)) {
				networks = append(networks, network{name: name, subnet: subnet, masquerade: masquerade})

				break
			}
		}
	}

	slices.SortFunc(networks, func(a network, b network) int {
		return strings.Compare(a.name, b.name)
	})

	return networks, nil
}

func find(networks []network, name string) (network, bool) {
	for _, n := range networks {
		if n.name == name {
			return n, true
		}
	}

	return network{}, false
}

// ports are the devices plugged into a bridge.
func ports(bridge netlink.Link) ([]string, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	var plugged []string
	for _, link := range links {
		if link.Attrs().MasterIndex == bridge.Attrs().Index {
			plugged = append(plugged, link.Attrs().Name)
		}
	}

	return plugged, nil
}

// deleteTaps takes away every tap of the fabric's that matches.
func deleteTaps(matches func(device string) bool) error {
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}

	var errs []error
	for _, link := range links {
		if isTap(link.Attrs().Name) && matches(link.Attrs().Name) {
			if err := netlink.LinkDel(link); err != nil {
				var gone netlink.LinkNotFoundError
				if !errors.As(err, &gone) {
					errs = append(errs, err)
				}
			}
		}
	}

	return errors.Join(errs...)
}

// forwarding makes sure machines' traffic is forwarded. A container is not
// always let change it for itself, so the holder container is made with it
// set, and the fabric only has to find it on.
func forwarding() error {
	const path = "/proc/sys/net/ipv4/ip_forward"

	if current, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(current)) == "1" {
		return nil
	}

	if err := os.WriteFile(path, []byte("1"), 0o644); err != nil {
		return fmt.Errorf("%w: machines' traffic is not forwarded, and cannot be made to be here: set net.ipv4.ip_forward=1 for the namespace's holder: %w", vm.ErrUnavailable, err)
	}

	return nil
}

// disableIPv6 keeps a device of the fabric's off IPv6, which nothing of the
// fabric's uses and the firewall does not cover. Where it cannot, IPv6 is off
// in the whole namespace already, and there is nothing to keep it off.
func disableIPv6(device string) {
	_ = os.WriteFile("/proc/sys/net/ipv6/conf/"+device+"/disable_ipv6", []byte("1"), 0o644)
}
