package firecracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
)

const (
	networksName = "networks.json"

	// detachTimeout is how long a stack's network is given to come free of
	// the machines being removed alongside it, and detachInterval how often
	// it is tried in the meantime.
	detachTimeout  = 30 * time.Second
	detachInterval = time.Second
)

// lease is a network as this orchestrator holds it: the subnet the launcher
// gave it, and which of its addresses are whose.
type lease struct {
	Subnet  string            `json:"subnet"`
	Gateway string            `json:"gateway"`
	Holders map[string]string `json:"holders"`
}

// Networks is an orchestrator's side of its machines' networks. The launcher
// makes them; which address each machine has is the orchestrator's to hand
// out, since every machine on them is its own.
type Networks struct {
	launcher machine.Launcher
	owner    string
	path     string

	lock   sync.Mutex
	leases map[string]*lease
}

var _ network.Manager = &Networks{}

// NewNetworks reads back what was handed out on an orchestrator's networks.
func NewNetworks(launcher machine.Launcher, owner string, dir string) (*Networks, error) {
	n := &Networks{
		launcher: launcher,
		owner:    owner,
		path:     filepath.Join(dir, networksName),
		leases:   make(map[string]*lease),
	}

	encoded, err := os.ReadFile(n.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if len(encoded) > 0 {
		if err := json.Unmarshal(encoded, &n.leases); err != nil {
			return nil, err
		}
	}

	return n, nil
}

// EnsureIsolatedNetwork makes the network standalone isolated tasks join, if
// it is not there already.
func (n *Networks) EnsureIsolatedNetwork(ctx context.Context) error {
	_, err := n.ensure(ctx, network.IsolatedNetworkName)

	return err
}

// EnsureStackNetwork makes the private network a stack's services share.
// Every service of a stack runs on one node, so it is this node's alone.
func (n *Networks) EnsureStackNetwork(ctx context.Context, stackSlug string) error {
	_, err := n.ensure(ctx, network.StackNetworkName(stackSlug))

	return err
}

// RemoveStackNetwork takes a stack's network away once its machines are off
// it. They are removed on the strength of one message and the network on
// another, so the network is often still holding them when this is asked for;
// it waits for them rather than leaving the network behind for good.
func (n *Networks) RemoveStackNetwork(ctx context.Context, stackSlug string) error {
	name := network.StackNetworkName(stackSlug)
	deadline := time.Now().Add(detachTimeout)

	for {
		err := n.remove(ctx, name)
		if err == nil {
			return nil
		}

		if !errors.Is(err, machine.ErrNetworkInUse) {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("the %q network still holds machines after %s: %w", name, detachTimeout, err)
		}

		select {
		case <-time.After(detachInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (n *Networks) remove(ctx context.Context, name string) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	if held, found := n.leases[name]; found && len(held.Holders) > 0 {
		return machine.ErrNetworkInUse
	}

	if err := n.launcher.RemoveNetwork(ctx, n.owner, name); err != nil {
		return err
	}

	delete(n.leases, name)

	return n.save()
}

// ensure makes a network, if it is not there already, and says what it is.
// The launcher is asked every time: a host that restarted has lost its
// bridges, and what this orchestrator remembers of them with it.
func (n *Networks) ensure(ctx context.Context, name string) (machine.Network, error) {
	made, err := n.launcher.EnsureNetwork(ctx, n.owner, name, name == network.PublicNetworkName)
	if err != nil {
		return machine.Network{}, err
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	held, found := n.leases[name]
	if !found || held.Subnet != made.Subnet {
		// a network made again is a new network: nothing handed out on the
		// one before it holds on this one.
		n.leases[name] = &lease{Subnet: made.Subnet, Gateway: made.Gateway, Holders: make(map[string]string)}

		if err := n.save(); err != nil {
			return machine.Network{}, err
		}
	}

	return made, nil
}

// lease hands one of a network's addresses to a machine, making the network
// if it is not there.
func (n *Networks) lease(ctx context.Context, attachment network.Attachment, id string) (iface, error) {
	made, err := n.ensure(ctx, attachment.Name)
	if err != nil {
		return iface{}, err
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	held := n.leases[attachment.Name]

	address, err := n.free(held, id)
	if err != nil {
		return iface{}, fmt.Errorf("%s: %w", attachment.Name, err)
	}

	held.Holders[address.String()] = id

	if err := n.save(); err != nil {
		return iface{}, err
	}

	_, subnet, err := net.ParseCIDR(made.Subnet)
	if err != nil {
		return iface{}, err
	}

	ones, _ := subnet.Mask.Size()

	leased := iface{
		Network: attachment.Name,
		Address: fmt.Sprintf("%s/%d", address, ones),
		MAC:     macFor(address),
		Aliases: attachment.Aliases,
	}

	if attachment.Gateway {
		leased.Gateway = made.Gateway
	}

	return leased, nil
}

// free is the first address of a network nobody holds, beyond the host's own.
// A machine asking again is given the one it already holds.
func (n *Networks) free(held *lease, id string) (net.IP, error) {
	for address, holder := range held.Holders {
		if holder == id {
			return net.ParseIP(address).To4(), nil
		}
	}

	_, subnet, err := net.ParseCIDR(held.Subnet)
	if err != nil {
		return nil, err
	}

	gateway := net.ParseIP(held.Gateway).To4()
	base := subnet.IP.To4()
	ones, bits := subnet.Mask.Size()

	for host := 2; host < 1<<(bits-ones)-1; host++ {
		candidate := make(net.IP, 4)
		copy(candidate, base)
		candidate[2] += byte(host >> 8)
		candidate[3] += byte(host & 0xff)

		if candidate.Equal(gateway) {
			continue
		}

		if _, taken := held.Holders[candidate.String()]; !taken {
			return candidate, nil
		}
	}

	return nil, errors.New("every address of the network is taken")
}

// release takes back whatever a machine holds on every network.
func (n *Networks) release(id string) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	for _, held := range n.leases {
		for address, holder := range held.Holders {
			if holder == id {
				delete(held.Holders, address)
			}
		}
	}

	return n.save()
}

// retain takes back whatever is held by a machine that is not among those
// still running, which is what is left of machines that went while nobody was
// looking.
func (n *Networks) retain(running map[string]bool) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	for _, held := range n.leases {
		for address, holder := range held.Holders {
			if !running[holder] {
				delete(held.Holders, address)
			}
		}
	}

	return n.save()
}

// save writes what has been handed out, in full, so it is never read half
// written.
func (n *Networks) save() error {
	encoded, err := json.Marshal(n.leases)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(n.path), 0o755); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(n.path), ".networks-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())

	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()

		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporary.Name(), n.path)
}

// macFor is the MAC a machine's device with address is given: a locally
// administered one, which is what a MAC nobody assigned has to be, carrying the
// address itself so that no two devices on a network share one.
func macFor(address net.IP) string {
	ip := address.To4()

	return fmt.Sprintf("06:00:%02x:%02x:%02x:%02x", ip[0], ip[1], ip[2], ip[3])
}

// cutPrefix takes an address in CIDR form apart.
func cutPrefix(address string) (string, string, bool) {
	return strings.Cut(address, "/")
}
