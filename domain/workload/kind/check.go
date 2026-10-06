package kind

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/permission"
)

var (
	// word is a kind's name, its plural, its parent and its actions' names.
	// They name routes, subjects and node requests, and a node request's op
	// is the kind and the action joined by a dot, so neither has one.
	word = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

	// verb is the last part of a permission, which domain/permission writes
	// in camel case and, now and then, in two parts: markAsRead,
	// password.update.
	verb = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*(\.[a-z][a-zA-Z0-9]*)*$`)
)

// Check holds a kind's descriptor to the rules every kind keeps, and is every
// rule it breaks. A kind that breaks none is one the framework can run, so a
// registry refuses any other, and the conformance test holds every kind the
// services register to the same rules in CI.
//
//   - Its names are words, its state is known on a node or in the control
//     plane, and its machine holds together (Machine.Validate).
//   - Its machine has failed, which is never in flight, and deleted, which is
//     terminal and left by nothing: the framework moves resources into both.
//   - A kind with a parent says what deleting and restoring the parent does
//     to it, and one without does not.
//   - Every action runs on a node or in the control plane, as a command, a
//     query or a stream; a stream runs on a node, and the control plane
//     answers no query but state, from the record. The states it is allowed
//     in and the one it desires are the machine's, and only a command desires
//     one. A transition on it is from a state it is allowed in, and is on a
//     command, since nothing else moves a resource.
//   - Every action has a permission, but an internal one, which only a
//     command can be.
//   - Every action has a codec, which decodes the zero value of its own
//     type.
//   - There is a state action, a query answered where the kind's state is
//     known, allowed in every state and asked with nothing; and a delete
//     action, a command that desires deleted.
//
// What needs the services' strategies is Services.Check, and what needs the
// permissions that exist is CheckPermissions.
func Check(d Descriptor) []error {
	var problems []error

	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("kind %q: "+format, append([]any{d.Name}, args...)...))
	}

	if !word.MatchString(d.Name) {
		add("its name is not a lowercase word")
	}

	if !word.MatchString(d.Plural) {
		add("its plural %q is not a lowercase word", d.Plural)
	}

	if !d.StateBy.IsValid() {
		add("its state is known neither on a node nor in the control plane (%q)", d.StateBy)
	}

	for _, problem := range d.Machine.Validate() {
		add("its machine: %w", problem)
	}

	checkFrameworkStates(d, add)
	checkParent(d, add)

	declared := make(map[string]int, len(d.Actions))
	for _, a := range d.Actions {
		declared[a.Name]++

		// said once, where it is declared the second time.
		if declared[a.Name] == 2 {
			add("action %q is declared more than once", a.Name)
		}

		checkAction(d, a, add)
	}

	checkTransitions(d, add)
	checkRequired(d, add)

	return problems
}

// checkFrameworkStates holds the machine to having the states the framework
// moves resources into itself.
func checkFrameworkStates(d Descriptor, add func(format string, args ...any)) {
	m := d.Machine

	if !m.Has(Failed) {
		add("its machine has no %q, which a resource whose command failed, or whose node is lost, is put in", Failed)
	} else if m.IsInFlight(Failed) {
		add("%q is in flight, and a failed resource is going nowhere", Failed)
	}

	if !m.Has(Deleted) {
		add("its machine has no %q, which deleting a resource desires", Deleted)
	} else if !m.IsTerminal(Deleted) {
		add("%q is not terminal, and a deleted resource is gone", Deleted)
	}

	for _, t := range m.Transitions {
		if t.From == Deleted {
			add("a transition on %s leaves %q, which nothing leaves", t.On, Deleted)
		}
	}
}

// checkParent holds a kind with a parent to saying what becomes of it when
// the parent is deleted or restored, and one without to saying nothing.
func checkParent(d Descriptor, add func(format string, args ...any)) {
	if len(d.Parent) == 0 {
		if d.OnParent != (ParentRules{}) {
			add("it has rules for a parent it does not have")
		}

		return
	}

	if !word.MatchString(d.Parent) {
		add("its parent %q is not a lowercase word", d.Parent)
	}

	if d.Parent == d.Name {
		add("it is its own parent")
	}

	switch d.OnParent.Delete {
	case CascadeDelete, CascadeKeep:
	case "":
		add("it has no rule for what deleting its %s does to it", d.Parent)
	default:
		add("%q is not what deleting its %s can do to it", d.OnParent.Delete, d.Parent)
	}

	switch d.OnParent.Restore {
	case CascadeDelete, CascadeKeep, CascadeReset:
	case "":
		add("it has no rule for what restoring its %s does to it", d.Parent)
	default:
		add("%q is not what restoring its %s can do to it", d.OnParent.Restore, d.Parent)
	}
}

// checkAction holds one action to the rules every action keeps.
func checkAction(d Descriptor, a Action, add func(format string, args ...any)) {
	if !word.MatchString(a.Name) {
		add("action %q: its name is not a lowercase word", a.Name)
	}

	if !a.Runs.IsValid() {
		add("action %q runs neither on a node nor in the control plane (%q)", a.Name, a.Runs)
	}

	if !a.Mode.IsValid() {
		add("action %q is neither a command, a query nor a stream (%q)", a.Name, a.Mode)
	}

	if a.Mode == ModeStream && a.Runs != OnNode {
		add("action %q is a stream, and only a node serves streams", a.Name)
	}

	if a.Mode == ModeQuery && a.Runs == OnControlPlane && a.Name != "state" {
		add("action %q is a query the control plane has nothing to answer with: it answers only state, from the record", a.Name)
	}

	for _, s := range a.AllowedIn {
		if !d.Machine.Has(s) {
			add("action %q is allowed in %q, which is not one of its states", a.Name, s)
		}
	}

	if len(a.Desires) > 0 {
		if a.Mode != ModeCommand {
			add("action %q desires %q, and only a command asks a resource to be anything", a.Name, a.Desires)
		}

		if !d.Machine.Has(a.Desires) {
			add("action %q desires %q, which is not one of its states", a.Name, a.Desires)
		}
	}

	switch {
	case a.Internal && len(a.Permission) > 0:
		add("action %q is internal, which nobody asks for, and has a permission", a.Name)
	case a.Internal && a.Mode != ModeCommand:
		add("action %q is internal, and only commands are the workload's own to ask for", a.Name)
	case !a.Internal && len(a.Permission) == 0:
		add("action %q has no permission: the verb of workload.%s.<verb> it is asked under", a.Name, d.Plural)
	case !a.Internal && !verb.MatchString(a.Permission):
		add("action %q is asked under %q, which is not a verb", a.Name, a.Permission)
	}

	if a.Payload == nil {
		add("action %q has no codec: NoPayload is the codec of an action asked with nothing", a.Name)
	} else if err := decodesZero(a.Payload); err != nil {
		add("action %q: %w", a.Name, err)
	}
}

// checkTransitions holds the transitions on actions to being on commands the
// kind has, from the states those are allowed in.
func checkTransitions(d Descriptor, add func(format string, args ...any)) {
	for _, t := range d.Machine.Transitions {
		if len(t.On.Action) == 0 {
			continue
		}

		a, found := d.Action(t.On.Action)
		if !found {
			add("a transition is on action %q, which it does not have", t.On.Action)

			continue
		}

		if a.Mode != ModeCommand {
			add("a transition is on action %q, a %s, and only a command moves a resource", a.Name, a.Mode)
		}

		if t.From != Any && len(a.AllowedIn) > 0 && !slices.Contains(a.AllowedIn, t.From) {
			add("a transition on action %q is from %q, where it is not allowed", a.Name, t.From)
		}
	}
}

// checkRequired holds the kind to having the actions the framework asks of
// every kind: state and delete.
func checkRequired(d Descriptor, add func(format string, args ...any)) {
	if state, found := d.Action("state"); !found {
		add("it has no state action, which is how what a resource is doing is known")
	} else {
		if state.Mode != ModeQuery {
			add("its state action is not a query")
		}

		if state.Runs != d.StateBy {
			add("its state action runs on the %s, and its state is known on the %s", state.Runs, d.StateBy)
		}

		if len(state.AllowedIn) > 0 {
			add("its state action is not allowed in every state")
		}

		if state.Payload != nil && state.Payload.Type() != nil {
			add("its state action is asked with a payload")
		}
	}

	if remove, found := d.Action("delete"); !found {
		add("it has no delete action, which is how every resource is removed")
	} else {
		if remove.Mode != ModeCommand {
			add("its delete action is not a command")
		}

		if remove.Desires != Deleted {
			add("its delete action desires %q rather than %q", remove.Desires, Deleted)
		}
	}
}

// decodesZero is why a codec cannot decode the zero value of its own type,
// or nil when it can: what its type writes, it has to be able to read. What
// is wrong with the zero value is no reason, since an empty payload is rarely
// a valid one.
func decodesZero(codec Codec) error {
	t := codec.Type()

	var raw []byte
	if t != nil {
		encoded, err := json.Marshal(reflect.Zero(t).Interface())
		if err != nil {
			return fmt.Errorf("its payload's zero value cannot be written: %w", err)
		}

		raw = encoded
	}

	value, _, err := codec.Decode(raw)
	if err != nil {
		return fmt.Errorf("its payload's zero value cannot be read: %w", err)
	}

	if t != nil && value != nil && reflect.TypeOf(value) != t {
		return fmt.Errorf("its payload decodes to a %T, not to the %v it says", value, t)
	}

	if t == nil && value != nil {
		return fmt.Errorf("it is asked with nothing, and nothing decodes to a %T", value)
	}

	return nil
}

// Services are every service's registry, which is what it takes to say
// whether every action of every kind has a strategy where it runs. A nil
// registry is one with nothing registered in it.
type Services struct {
	ControlPlane *Registry[ControlPlaneBinding]
	Node         *Registry[NodeBinding]
	Ingress      *Registry[IngressBinding]
}

// Descriptors are the descriptors of every kind registered in any of the
// services, once each: the control plane's first, then the nodes', then the
// ingress's, each in the order they were registered.
func (s Services) Descriptors() []Descriptor {
	var descriptors []Descriptor

	seen := make(map[string]bool)

	for _, registered := range [][]Descriptor{
		s.ControlPlane.Descriptors(),
		s.Node.Descriptors(),
		s.Ingress.Descriptors(),
	} {
		for _, d := range registered {
			if !seen[d.Name] {
				seen[d.Name] = true
				descriptors = append(descriptors, d)
			}
		}
	}

	return descriptors
}

// Check is every rule the kinds registered across the services break
// between them:
//
//   - every kind is registered in the control plane, which keeps every kind;
//   - a kind registered in several services is described the same in each;
//   - every action that runs on a node has a node strategy to run it, and a
//     stream one that attaches;
//   - a kind with endpoints has a node strategy that serves its ports;
//   - a kind reached through the ingress, by its endpoints or its streams,
//     has an ingress strategy to find its instances.
func (s Services) Check() []error {
	var problems []error

	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	for _, d := range s.Descriptors() {
		controlPlane, inControlPlane := s.ControlPlane.Lookup(d.Name)
		node, onNodes := s.Node.Lookup(d.Name)
		ingress, inIngress := s.Ingress.Lookup(d.Name)

		if !inControlPlane {
			add("kind %q is not registered in the control plane, which keeps every kind", d.Name)
		}

		described := map[string]Binding{}
		if inControlPlane {
			described["control plane"] = controlPlane
		}

		if onNodes {
			described["nodes"] = node
		}

		if inIngress {
			described["ingress"] = ingress
		}

		for _, service := range []string{"nodes", "ingress"} {
			if binding, registered := described[service]; registered && !reflect.DeepEqual(binding.Descriptor(), d) {
				add("kind %q is described in the %s otherwise than it is elsewhere", d.Name, service)
			}
		}

		reached := d.Endpoints

		for _, a := range d.Actions {
			if a.Mode == ModeStream {
				reached = true
			}

			if a.Runs != OnNode {
				continue
			}

			switch {
			case !onNodes:
				add("kind %q: action %q runs on a node, and no node strategy is registered for it", d.Name, a.Name)
			case a.Mode == ModeStream && !node.Attaches():
				add("kind %q: action %q is a stream, and its node strategy serves none", d.Name, a.Name)
			}
		}

		if d.Endpoints && (!onNodes || !node.Exposes()) {
			add("kind %q has endpoints, and no node strategy serves its ports", d.Name)
		}

		if reached && !inIngress {
			add("kind %q is reached through the ingress, and no ingress strategy is registered for it", d.Name)
		}
	}

	return problems
}

// CheckPermissions is every permission a kind's actions are asked under that
// does not exist, given every permission there is, as the roles page lists
// them: one that is not listed is one no role can be given, and one listed
// without a name is one nobody can tell what it grants. Each is said once,
// however many actions share it.
func CheckPermissions(d Descriptor, listed []permission.Permission) []error {
	names := make(map[string]string, len(listed))
	for _, p := range listed {
		names[p.Value] = p.Name
	}

	var problems []error

	said := make(map[string]bool)

	for _, a := range d.Actions {
		if a.Internal || len(a.Permission) == 0 {
			continue
		}

		admin, self := d.Permissions(a.Permission)

		for _, p := range []string{admin, self} {
			if said[p] {
				continue
			}

			said[p] = true

			name, exists := names[p]

			switch {
			case !exists:
				problems = append(problems, fmt.Errorf("kind %q: action %q is asked under %q, which is not a permission", d.Name, a.Name, p))
			case len(name) == 0:
				problems = append(problems, fmt.Errorf("kind %q: action %q is asked under %q, which is listed without saying what it grants", d.Name, a.Name, p))
			}
		}
	}

	return problems
}
