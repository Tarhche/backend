package kind

import (
	"errors"
	"fmt"
	"slices"
	"sync"
)

// Registry is the kinds one service runs, each as its binding there: the
// control plane's hold ControlPlaneBindings, an orchestrator's NodeBindings,
// the ingress's IngressBindings. Every service runs one generic loop over its
// registry, so a kind is added to a service by one registration.
//
// Kinds are registered while a service is put together and looked up while
// it serves, so it is safe for concurrent use.
type Registry[B Binding] struct {
	mutex    sync.RWMutex
	bindings []B
	byName   map[string]int
	byPlural map[string]int
}

// NewRegistry is a registry with no kind in it yet.
func NewRegistry[B Binding]() *Registry[B] {
	return &Registry[B]{
		byName:   make(map[string]int),
		byPlural: make(map[string]int),
	}
}

// Register adds a kind. It refuses one whose descriptor breaks a rule every
// kind keeps (Check), so a kind that cannot run is found out when its service
// is put together rather than in production, and one whose name or plural
// another kind has already.
func (r *Registry[B]) Register(binding B) error {
	if any(binding) == nil {
		return errors.New("a kind cannot be registered without a binding")
	}

	d := binding.Descriptor()

	if problems := Check(d); len(problems) > 0 {
		return fmt.Errorf("a kind that breaks a rule cannot be registered: %w", errors.Join(problems...))
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, taken := r.byName[d.Name]; taken {
		return fmt.Errorf("kind %q is registered already", d.Name)
	}

	if index, taken := r.byPlural[d.Plural]; taken {
		return fmt.Errorf("kind %q cannot be registered: %q is the plural of %q already", d.Name, d.Plural, r.bindings[index].Descriptor().Name)
	}

	r.bindings = append(r.bindings, binding)
	r.byName[d.Name] = len(r.bindings) - 1
	r.byPlural[d.Plural] = len(r.bindings) - 1

	return nil
}

// Lookup is the kind registered under name, which is how messages and node
// requests name it.
func (r *Registry[B]) Lookup(name string) (B, bool) {
	return r.find(func() (int, bool) {
		index, found := r.byName[name]

		return index, found
	})
}

// ByPlural is the kind registered under plural, which is how routes name it.
func (r *Registry[B]) ByPlural(plural string) (B, bool) {
	return r.find(func() (int, bool) {
		index, found := r.byPlural[plural]

		return index, found
	})
}

// All is every kind registered, in the order they were: the ingress asks
// them for a slug in that order.
func (r *Registry[B]) All() []B {
	if r == nil {
		return nil
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return slices.Clone(r.bindings)
}

// Descriptors are the descriptors of every kind registered, in the order
// they were.
func (r *Registry[B]) Descriptors() []Descriptor {
	all := r.All()

	descriptors := make([]Descriptor, len(all))
	for i, binding := range all {
		descriptors[i] = binding.Descriptor()
	}

	return descriptors
}

// find is the binding at the index index says, read under the lock. A nil
// registry has nothing in it.
func (r *Registry[B]) find(index func() (int, bool)) (B, bool) {
	var none B

	if r == nil {
		return none, false
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	i, found := index()
	if !found {
		return none, false
	}

	return r.bindings[i], true
}
