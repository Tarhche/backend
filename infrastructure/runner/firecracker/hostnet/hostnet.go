//go:build linux

package hostnet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// Config is what machines' networks are made of.
type Config struct {
	// Pool is where every network's /24 comes from. It must be the runner's
	// alone: the firewall keeps it apart from everything else.
	Pool *net.IPNet

	// FirstUID and UIDs are the users machines run as. Nothing they send
	// from the launcher's own address goes anywhere.
	FirstUID int
	UIDs     int
}

// HostNetwork makes the bridges and taps machines are plugged into, and the
// firewall that says what may reach what. It does all of it in the network
// namespace the launcher runs in, which is its container's own: the host's
// devices and firewall are never touched.
type HostNetwork struct {
	config Config
	logger *slog.Logger

	// lock serialises changes: allocating a /24 and rebuilding the firewall
	// both read what is there before they change it.
	lock sync.Mutex
}

var _ machine.HostNetwork = &HostNetwork{}

// New prepares for the runner's networks: forwarding on, and the firewall as
// the networks already there say it should be.
func New(config Config, logger *slog.Logger) (*HostNetwork, error) {
	h := &HostNetwork{config: config, logger: logger}

	if err := forwarding(); err != nil {
		return nil, err
	}

	networks, err := h.networks()
	if err != nil {
		return nil, err
	}

	if err := h.applyFirewall(networks); err != nil {
		return nil, err
	}

	return h, nil
}

func (h *HostNetwork) EnsureNetwork(ctx context.Context, owner string, name string, masquerade bool) (machine.Network, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	networks, err := h.networks()
	if err != nil {
		return machine.Network{}, err
	}

	for _, n := range networks {
		if n.owner == owner && n.name == name {
			if n.masquerade != masquerade {
				return machine.Network{}, fmt.Errorf("%s is already a network, and routing out is not something it can start or stop doing", name)
			}

			return toNetwork(n), nil
		}
	}

	taken := make([]*net.IPNet, 0, len(networks))
	for _, n := range networks {
		taken = append(taken, n.subnet)
	}

	subnet, err := allocate(h.config.Pool, taken)
	if err != nil {
		return machine.Network{}, err
	}

	made, err := h.makeBridge(owner, name, subnet, masquerade)
	if err != nil {
		return machine.Network{}, err
	}

	if err := h.applyFirewall(append(networks, made)); err != nil {
		return machine.Network{}, err
	}

	h.logger.Info("network made", "owner", owner, "network", name, "subnet", subnet.String(), "masquerade", masquerade)

	return toNetwork(made), nil
}

func (h *HostNetwork) makeBridge(owner string, name string, subnet *net.IPNet, masquerade bool) (networkState, error) {
	bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName(owner, name)}}

	if err := netlink.LinkAdd(bridge); err != nil {
		return networkState{}, fmt.Errorf("failed to make the bridge for %s: %w", name, err)
	}

	link, err := netlink.LinkByName(bridge.Name)
	if err != nil {
		return networkState{}, err
	}

	fail := func(err error) (networkState, error) {
		return networkState{}, errors.Join(err, netlink.LinkDel(link))
	}

	if err := netlink.LinkSetAlias(link, alias(owner, name, masquerade)); err != nil {
		return fail(err)
	}

	address := &netlink.Addr{IPNet: &net.IPNet{IP: gateway(subnet), Mask: subnet.Mask}}
	if err := netlink.AddrAdd(link, address); err != nil {
		return fail(err)
	}

	disableIPv6(bridge.Name)

	if err := netlink.LinkSetUp(link); err != nil {
		return fail(err)
	}

	return networkState{owner: owner, name: name, subnet: subnet, masquerade: masquerade}, nil
}

func (h *HostNetwork) RemoveNetwork(ctx context.Context, owner string, name string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	link, err := netlink.LinkByName(bridgeName(owner, name))
	if err != nil {
		var notFound netlink.LinkNotFoundError
		if errors.As(err, &notFound) {
			return nil
		}

		return err
	}

	links, err := netlink.LinkList()
	if err != nil {
		return err
	}

	for _, other := range links {
		if other.Attrs().MasterIndex == link.Attrs().Index {
			return machine.ErrNetworkInUse
		}
	}

	if err := netlink.LinkDel(link); err != nil {
		return err
	}

	networks, err := h.networks()
	if err != nil {
		return err
	}

	h.logger.Info("network removed", "owner", owner, "network", name)

	return h.applyFirewall(networks)
}

func (h *HostNetwork) Plug(ctx context.Context, owner string, id string, user int, taps []machine.Tap) ([]machine.AttachedTap, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	attached := make([]machine.AttachedTap, 0, len(taps))

	for i, tap := range taps {
		bridge, err := netlink.LinkByName(bridgeName(owner, tap.Network))
		if err != nil {
			return attached, fmt.Errorf("%s is not one of %s's networks: %w", tap.Network, owner, err)
		}

		_, _, masquerade, _ := parseAlias(bridge.Attrs().Alias)

		device := tapName(id, i)

		// a tap left by an earlier attempt at the same machine is taken
		// away first, so what is plugged in is what was asked for now.
		if stale, err := netlink.LinkByName(device); err == nil {
			_ = netlink.LinkDel(stale)
		}

		// a tap is its machine's user's alone to open, which is what lets the
		// machine's firecracker open it without any privilege at all.
		created := &netlink.Tuntap{
			LinkAttrs: netlink.LinkAttrs{Name: device},
			Mode:      netlink.TUNTAP_MODE_TAP,
			Flags:     netlink.TUNTAP_NO_PI,
			Owner:     uint32(user),
			Group:     uint32(user),
		}

		if err := netlink.LinkAdd(created); err != nil {
			return attached, fmt.Errorf("failed to make the tap for %s: %w", tap.Network, err)
		}

		link, err := netlink.LinkByName(device)
		if err != nil {
			return attached, err
		}

		if err := netlink.LinkSetMaster(link, bridge); err != nil {
			return attached, err
		}

		// machines on a public network do not reach each other: the network
		// is there to route out, and gives them nothing else in common.
		if masquerade {
			if err := netlink.LinkSetIsolated(link, true); err != nil {
				return attached, err
			}
		}

		disableIPv6(device)

		if err := netlink.LinkSetUp(link); err != nil {
			return attached, err
		}

		attached = append(attached, machine.AttachedTap{Network: tap.Network, Device: device})
	}

	return attached, nil
}

func (h *HostNetwork) Unplug(ctx context.Context, id string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	links, err := netlink.LinkList()
	if err != nil {
		return err
	}

	prefix := tapPrefixOf(id)

	var errs []error
	for _, link := range links {
		if strings.HasPrefix(link.Attrs().Name, prefix) {
			errs = append(errs, netlink.LinkDel(link))
		}
	}

	return errors.Join(errs...)
}

// networks is every network of the runner's the kernel holds, read off the
// bridges themselves.
func (h *HostNetwork) networks() ([]networkState, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	var networks []networkState

	for _, link := range links {
		if link.Type() != "bridge" || !strings.HasPrefix(link.Attrs().Name, bridgePrefix) {
			continue
		}

		owner, name, masquerade, ok := parseAlias(link.Attrs().Alias)
		if !ok {
			continue
		}

		addresses, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil || len(addresses) == 0 {
			continue
		}

		subnet := &net.IPNet{IP: addresses[0].IP.Mask(addresses[0].Mask), Mask: addresses[0].Mask}

		networks = append(networks, networkState{owner: owner, name: name, subnet: subnet, masquerade: masquerade})
	}

	return networks, nil
}

// applyFirewall writes the runner's chains again from the networks there are.
func (h *HostNetwork) applyFirewall(networks []networkState) error {
	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("the firewall cannot be reached: %w", err)
	}

	for _, chain := range []struct{ table, name string }{
		{"filter", forwardChain},
		{"filter", inputChain},
		{"filter", outputChain},
		{"nat", postroutingChain},
	} {
		if err := ipt.ClearChain(chain.table, chain.name); err != nil {
			return err
		}
	}

	for _, r := range rules(h.config.Pool, networks, users{first: h.config.FirstUID, count: h.config.UIDs}) {
		if err := ipt.Append(r.table, r.chain, r.spec...); err != nil {
			return err
		}
	}

	dockerUser, err := ipt.ChainExists("filter", dockerUserChain)
	if err != nil {
		return err
	}

	for _, j := range jumps(dockerUser) {
		exists, err := ipt.Exists(j.table, j.from, "-j", j.to)
		if err != nil {
			return err
		}

		if !exists {
			if err := ipt.Insert(j.table, j.from, 1, "-j", j.to); err != nil {
				return err
			}
		}
	}

	return nil
}

func toNetwork(n networkState) machine.Network {
	return machine.Network{
		Owner:      n.owner,
		Name:       n.name,
		Subnet:     n.subnet.String(),
		Gateway:    gateway(n.subnet).String(),
		Masquerade: n.masquerade,
	}
}

// forwarding makes sure machines' traffic is forwarded. A container is not let
// change it for itself — its /proc/sys is read only — so a launcher in one
// has it set for the container, and only has to find it on.
func forwarding() error {
	const path = "/proc/sys/net/ipv4/ip_forward"

	if current, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(current)) == "1" {
		return nil
	}

	if err := os.WriteFile(path, []byte("1"), 0o644); err != nil {
		return fmt.Errorf("machines' traffic is not forwarded, and cannot be made to be here: set net.ipv4.ip_forward=1 for the launcher's container: %w", err)
	}

	return nil
}

// disableIPv6 keeps a runner device off IPv6, which nothing of the runner's
// uses and the firewall does not cover. A container cannot do this for itself,
// and is set to make every device without IPv6 instead.
func disableIPv6(device string) {
	_ = os.WriteFile("/proc/sys/net/ipv6/conf/"+device+"/disable_ipv6", []byte("1"), 0o644)
}
