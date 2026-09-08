package task

import (
	"errors"
	"strconv"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

// PortRange is the span of ports the ingress accepts raw connections on.
//
// A container that exposes a port is given one of these to be reached at, so
// that what does not speak http — ssh, a database client — has an address. An
// empty range gives out none, and the containers are served over http alone.
type PortRange struct {
	First port.Port
	Last  port.Port
}

// ParsePortRange reads a range written as "first-last". Nothing written down
// is no range at all rather than an error: a runner that forwards nothing is a
// runner, and says so this way.
func ParsePortRange(value string) (PortRange, error) {
	trimmed := strings.TrimSpace(value)

	if trimmed == "" {
		return PortRange{}, nil
	}

	first, last, found := strings.Cut(trimmed, "-")
	if !found {
		return PortRange{}, errors.New("a port range is written as \"first-last\"")
	}

	from, err := strconv.ParseUint(strings.TrimSpace(first), 10, 16)
	if err != nil {
		return PortRange{}, err
	}

	to, err := strconv.ParseUint(strings.TrimSpace(last), 10, 16)
	if err != nil {
		return PortRange{}, err
	}

	if from == 0 || to < from {
		return PortRange{}, errors.New("a port range runs from a port to a higher one")
	}

	return PortRange{First: port.Port(from), Last: port.Port(to)}, nil
}

// Empty reports whether the range gives out no ports at all.
func (r PortRange) Empty() bool {
	return r.First == 0 || r.Last < r.First
}

// Free is the lowest port in the range that nothing has taken.
func (r PortRange) Free(taken map[port.Port]struct{}) (port.Port, bool) {
	for candidate := r.First; candidate <= r.Last; candidate++ {
		if _, ok := taken[candidate]; !ok {
			return candidate, true
		}
	}

	return 0, false
}
