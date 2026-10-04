// Package hostports hands out the host ports the workload-microsandbox
// service publishes runs' ports on.
//
// Microsandbox fixes a sandbox's published ports when it makes the sandbox,
// and whether it would pick free ones itself is untested, so the service picks
// them: from a range of its own, one for each port a run publishes, at the
// run's first boot, kept for as long as the run exists.
package hostports

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

// DefaultQuarantine is how long a deleted run's ports are kept from other
// runs. A published port's listener can outlive its sandbox (microsandbox
// #1717), and a port handed to a new run while the old listener holds it
// would be a run that cannot boot.
const DefaultQuarantine = 5 * time.Minute

// Allocator hands out the ports of one range, on one address.
type Allocator struct {
	first, last uint16
	bind        string
	quarantine  time.Duration

	mu sync.Mutex

	// held are the ports runs have, and runs which ports each has.
	held map[uint16]string
	runs map[string][]uint16

	// released are ports let go of, and when they may be handed out again.
	released map[uint16]time.Time

	// next is where the next search for a free port begins, so that ports
	// are handed out around the range rather than the lowest again and
	// again, and one let go of is the last to be handed out again.
	next uint16

	now func() time.Time

	// free is whether nothing is listening on an address, which a test bind
	// finds out.
	free func(address string) bool
}

var _ runs.HostPorts = &Allocator{}

// New hands out the ports from first to last, both included, checking each on
// bind before handing it out.
func New(first, last uint16, bind string) (*Allocator, error) {
	if first == 0 || last < first {
		return nil, fmt.Errorf("hostports: %d-%d is not a range of ports", first, last)
	}

	if net.ParseIP(bind) == nil {
		return nil, fmt.Errorf("hostports: %q is not an address to bind", bind)
	}

	return &Allocator{
		first:      first,
		last:       last,
		bind:       bind,
		quarantine: DefaultQuarantine,
		held:       make(map[uint16]string),
		runs:       make(map[string][]uint16),
		released:   make(map[uint16]time.Time),
		next:       first,
		now:        time.Now,
		free:       testBind,
	}, nil
}

// Allocate picks count free ports for a run.
//
// A port is free when no run holds it, it is out of quarantine, and nothing is
// listening on it: a test bind catches what the records cannot know of, such
// as a listener a sandbox left behind.
func (a *Allocator) Allocate(id string, count int) ([]uint16, error) {
	if count <= 0 {
		return nil, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.now()
	size := int(a.last) - int(a.first) + 1
	ports := make([]uint16, 0, count)

	for tried := 0; tried < size && len(ports) < count; tried++ {
		port := a.next

		a.next++
		if a.next > a.last || a.next < a.first {
			a.next = a.first
		}

		if _, taken := a.held[port]; taken {
			continue
		}

		if until, quarantined := a.released[port]; quarantined {
			if now.Before(until) {
				continue
			}

			delete(a.released, port)
		}

		if !a.free(net.JoinHostPort(a.bind, strconv.Itoa(int(port)))) {
			continue
		}

		ports = append(ports, port)
	}

	if len(ports) < count {
		return nil, fmt.Errorf("only %d of the %d host ports asked for are free in %d-%d", len(ports), count, a.first, a.last)
	}

	for _, port := range ports {
		a.held[port] = id
	}

	a.runs[id] = append(a.runs[id], ports...)

	return ports, nil
}

// Hold takes ports a run already has, as its record says. A port outside the
// range is held all the same, since the run has it: the range may have
// changed since.
func (a *Allocator) Hold(id string, ports []uint16) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, port := range ports {
		a.held[port] = id
		delete(a.released, port)
	}

	a.runs[id] = append(a.runs[id], ports...)
}

// Release lets go of a run's ports, which are quarantined before they are
// handed out again.
func (a *Allocator) Release(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	until := a.now().Add(a.quarantine)

	for _, port := range a.runs[id] {
		if a.held[port] == id {
			delete(a.held, port)
			a.released[port] = until
		}
	}

	delete(a.runs, id)
}

// testBind is whether an address can be listened on, which it cannot while
// something else listens there.
func testBind(address string) bool {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}

	_ = listener.Close()

	return true
}
