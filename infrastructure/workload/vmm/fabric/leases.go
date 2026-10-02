package fabric

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// leasesName is the file the addresses handed out are kept in.
const leasesName = "leases.json"

// lease is one network's addresses as the fabric handed them out: the subnet
// the network had when it did, and whose each address is.
type lease struct {
	Subnet  string            `json:"subnet"`
	Holders map[string]string `json:"holders,omitempty"`
}

// leases are the addresses handed out on every network, by network name. They
// are written down in the fabric's directory, because a machine keeps its
// address across a vmhost restart, and the next vmhost must not hand it to
// another.
//
// They are also what remembers the subnet a network had: one made again after
// the host restarted, and its bridges with it, is given the same one if it is
// free, so that nothing that knew the network's addresses is surprised.
type leases struct {
	path     string
	networks map[string]*lease
}

// openLeases reads back the addresses handed out before.
func openLeases(dir string) (*leases, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	l := &leases{path: filepath.Join(dir, leasesName), networks: map[string]*lease{}}

	encoded, err := os.ReadFile(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(encoded, &l.networks); err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", l.path, err)
	}

	for name, held := range l.networks {
		if held == nil {
			delete(l.networks, name)

			continue
		}

		if held.Holders == nil {
			held.Holders = map[string]string{}
		}
	}

	return l, nil
}

// on is a network's lease as the network is now. A network made again on
// another subnet is a new network: nothing handed out on the one before holds
// on it.
func (l *leases) on(network string, subnet *net.IPNet) *lease {
	held, found := l.networks[network]
	if !found || held.Subnet != subnet.String() {
		held = &lease{Subnet: subnet.String(), Holders: map[string]string{}}
		l.networks[network] = held
	}

	return held
}

// hand gives a machine an address on a network: the one it holds already,
// if it holds one, or the first nobody does after the fabric's own.
func (l *leases) hand(network string, subnet *net.IPNet, id string) (net.IP, error) {
	held := l.on(network, subnet)

	for address, holder := range held.Holders {
		if holder == id {
			return net.ParseIP(address).To4(), nil
		}
	}

	base := binary.BigEndian.Uint32(subnet.IP.To4())
	ones, bits := subnet.Mask.Size()
	own := binary.BigEndian.Uint32(gateway(subnet))

	// the network's own address and its broadcast are nobody's, and the
	// first after it is the fabric's.
	for host := uint32(1); host < 1<<(bits-ones)-1; host++ {
		if base+host == own {
			continue
		}

		candidate := make(net.IP, net.IPv4len)
		binary.BigEndian.PutUint32(candidate, base+host)

		if _, taken := held.Holders[candidate.String()]; !taken {
			held.Holders[candidate.String()] = id

			return candidate, nil
		}
	}

	return nil, fmt.Errorf("%w: every address of %s is taken", vm.ErrCapacity, network)
}

// keepOnly gives back what a machine holds on every network but those named.
func (l *leases) keepOnly(id string, networks []string) {
	for name, held := range l.networks {
		if slices.Contains(networks, name) {
			continue
		}

		maps.DeleteFunc(held.Holders, func(_ string, holder string) bool {
			return holder == id
		})
	}
}

// release gives back whatever a machine holds on every network.
func (l *leases) release(id string) {
	l.keepOnly(id, nil)
}

// retain gives back whatever is held by a machine that is not kept, and says
// which machines those were.
func (l *leases) retain(kept func(id string) bool) []string {
	var released []string

	for _, held := range l.networks {
		maps.DeleteFunc(held.Holders, func(_ string, holder string) bool {
			if kept(holder) {
				return false
			}

			if !slices.Contains(released, holder) {
				released = append(released, holder)
			}

			return true
		})
	}

	slices.Sort(released)

	return released
}

// inUse reports whether anything holds an address on a network.
func (l *leases) inUse(network string) bool {
	held, found := l.networks[network]

	return found && len(held.Holders) > 0
}

// subnetOf is the subnet a network had when it was last made.
func (l *leases) subnetOf(network string) *net.IPNet {
	held, found := l.networks[network]
	if !found {
		return nil
	}

	_, subnet, err := net.ParseCIDR(held.Subnet)
	if err != nil {
		return nil
	}

	return subnet
}

// forget lets go of a network that has been taken away.
func (l *leases) forget(network string) {
	delete(l.networks, network)
}

// save writes what has been handed out, in full, so it is never read half
// written.
func (l *leases) save() error {
	encoded, err := json.MarshalIndent(l.networks, "", "  ")
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(l.path), ".leases-")
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

	return os.Rename(temporary.Name(), l.path)
}
