package microsandbox

import (
	"fmt"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Where host ports are given from when the options name no range, as the
// vmhost's own default does.
const (
	defaultFirstPort port.Port = 20000
	defaultLastPort  port.Port = 29999
)

// portRange is the host ports a node gives published guest ports.
type portRange struct {
	first port.Port
	last  port.Port
}

func newPortRange(first port.Port, last port.Port) (portRange, error) {
	if first == 0 && last == 0 {
		return portRange{first: defaultFirstPort, last: defaultLastPort}, nil
	}

	if first == 0 || last < first || last > 65535 {
		return portRange{}, fmt.Errorf("host ports %d-%d are not a range of ports", first, last)
	}

	return portRange{first: first, last: last}, nil
}

// assign gives every guest port a host port.
//
// A guest port keeps the host port it had, so an instance that is restarted,
// restored or reconfigured is dialled where it was; one it no longer has is
// let go of, and a new one gets the lowest host port nobody else has and that
// nothing else in the container is listening on. taken is every host port
// given to other instances, and available says whether a port can still be
// bound, which a port something else holds cannot.
func (r portRange) assign(guests []port.Port, had map[port.Port]port.Port, taken map[port.Port]bool, available func(port.Port) bool) (map[port.Port]port.Port, error) {
	given := make(map[port.Port]port.Port, len(guests))
	used := make(map[port.Port]bool, len(taken)+len(guests))

	for host := range taken {
		used[host] = true
	}

	for _, guest := range guests {
		if guest == 0 || guest > 65535 {
			return nil, fmt.Errorf("%d is not a port", guest)
		}

		if host, ok := had[guest]; ok && !used[host] {
			given[guest] = host
			used[host] = true
		}
	}

	next := r.first

	for _, guest := range sortedUnique(guests) {
		if _, ok := given[guest]; ok {
			continue
		}

		for ; next <= r.last && (used[next] || !available(next)); next++ {
		}

		if next > r.last {
			return nil, fmt.Errorf("%w: no host port is left in %d-%d", vm.ErrNoCapacity, r.first, r.last)
		}

		given[guest] = next
		used[next] = true
		next++
	}

	return given, nil
}

func sortedUnique(ports []port.Port) []port.Port {
	sorted := slices.Clone(ports)
	slices.Sort(sorted)

	return slices.Compact(sorted)
}
