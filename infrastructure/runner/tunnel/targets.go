package tunnel

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
)

var (
	// ErrUnknownService is a name the worker does not offer.
	ErrUnknownService = errors.New("tunnel: no such service")

	// ErrTargetNotAllowed is an address the worker will not connect to. A
	// tunnel whose far end will dial anything is a way into everything that
	// end can reach.
	ErrTargetNotAllowed = errors.New("tunnel: target is not allowed")
)

// Targets is what a worker will connect a stream to.
//
// It is the whole of the worker's authorisation: the ingress asks, and this
// decides. Naming a service rather than an address is what lets a worker offer
// several things — an api, a database, a container's port — without the ingress
// knowing what any of them are or where they live, and lets them move without
// anything else being told.
type Targets interface {
	// Resolve turns what was asked for into a host:port to dial, or says why
	// it will not.
	Resolve(ctx context.Context, target Target) (string, error)
}

// TargetsFunc adapts a function to Targets.
type TargetsFunc func(ctx context.Context, target Target) (string, error)

func (f TargetsFunc) Resolve(ctx context.Context, target Target) (string, error) {
	return f(ctx, target)
}

// ServiceTargets offers a fixed set of named services, and optionally a set of
// addresses that may be asked for directly.
//
// A worker that offers only names cannot be talked into connecting anywhere
// else at all, which is the safe default; allowing addresses is for the cases
// where the caller genuinely has to choose the port, and is bounded by what is
// allowed rather than left open.
type ServiceTargets struct {
	lock     sync.RWMutex
	services map[string]string
	allowed  []AddressRule
}

var _ Targets = &ServiceTargets{}

// AddressRule is a host, and the ports on it that may be asked for.
type AddressRule struct {
	// Host is the host that may be dialled, matched exactly.
	Host string

	// Ports are the ports allowed on it. Empty allows none; use PortRange for
	// a span.
	Ports []uint16

	// From and To allow a span of ports, inclusive. Zero to zero allows none.
	From uint16
	To   uint16
}

func (r AddressRule) allows(host string, port uint16) bool {
	if !strings.EqualFold(r.Host, host) {
		return false
	}

	if slices.Contains(r.Ports, port) {
		return true
	}

	return r.From > 0 && port >= r.From && port <= r.To
}

// ParseAddressRules reads a comma-separated list of addresses a worker will
// connect a stream to, each a single port or a span of them:
//
//	127.0.0.1:5432              one port
//	127.0.0.1:30000-31000       a span, which is how published container ports
//	                            are allowed without naming each one
//
// Nothing is allowed by default. What is not listed here a worker will not
// dial, whatever the ingress asks for.
func ParseAddressRules(rules string) ([]AddressRule, error) {
	allowed := make([]AddressRule, 0, 1)

	for _, rule := range strings.Split(rules, ",") {
		if rule = strings.TrimSpace(rule); len(rule) == 0 {
			continue
		}

		parsed, err := ParseAddressRule(rule)
		if err != nil {
			return nil, err
		}

		allowed = append(allowed, parsed)
	}

	return allowed, nil
}

// ParseAddressRule reads one allowed address, as host:port or host:from-to.
func ParseAddressRule(rule string) (AddressRule, error) {
	host, ports, err := net.SplitHostPort(strings.TrimSpace(rule))
	if err != nil {
		return AddressRule{}, fmt.Errorf("%w: %q is not an address: %s", ErrTargetNotAllowed, rule, err)
	}

	if len(host) == 0 {
		return AddressRule{}, fmt.Errorf("%w: %q names no host", ErrTargetNotAllowed, rule)
	}

	from, to, spanned := strings.Cut(ports, "-")
	if !spanned {
		port, err := portNumber(from)
		if err != nil {
			return AddressRule{}, err
		}

		return AddressRule{Host: host, Ports: []uint16{port}}, nil
	}

	first, err := portNumber(from)
	if err != nil {
		return AddressRule{}, err
	}

	last, err := portNumber(to)
	if err != nil {
		return AddressRule{}, err
	}

	if last < first {
		return AddressRule{}, fmt.Errorf("%w: %q ends before it starts", ErrTargetNotAllowed, rule)
	}

	return AddressRule{Host: host, From: first, To: last}, nil
}

func portNumber(port string) (uint16, error) {
	number, err := strconv.ParseUint(strings.TrimSpace(port), 10, 16)
	if err != nil || number == 0 {
		return 0, fmt.Errorf("%w: %q is not a port", ErrTargetNotAllowed, port)
	}

	return uint16(number), nil
}

// NewServiceTargets builds the set of things a worker offers, keyed by the name
// the ingress will ask for.
func NewServiceTargets(services map[string]string, allowed ...AddressRule) *ServiceTargets {
	return &ServiceTargets{services: maps.Clone(services), allowed: allowed}
}

// Set adds or replaces a service while the worker is running, which is how one
// that comes to hold something new starts offering it.
func (t *ServiceTargets) Set(name string, address string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.services == nil {
		t.services = make(map[string]string)
	}

	t.services[name] = address
}

// Remove stops offering a service.
func (t *ServiceTargets) Remove(name string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.services, name)
}

func (t *ServiceTargets) Resolve(_ context.Context, target Target) (string, error) {
	if target.Named() {
		t.lock.RLock()
		address, ok := t.services[target.Service]
		t.lock.RUnlock()

		if !ok {
			return "", fmt.Errorf("%w: %s", ErrUnknownService, target.Service)
		}

		return address, nil
	}

	if len(target.Host) == 0 || target.Port == 0 {
		return "", errors.Join(ErrTargetNotAllowed, errors.New("no target"))
	}

	for _, rule := range t.allowed {
		if rule.allows(target.Host, target.Port) {
			return net.JoinHostPort(target.Host, fmt.Sprint(target.Port)), nil
		}
	}

	return "", fmt.Errorf("%w: %s", ErrTargetNotAllowed, target)
}
