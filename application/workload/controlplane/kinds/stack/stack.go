// Package stack is the stack kind's control-plane strategy, and the stack as
// a graph needs it (Graph): what a stack is admitted as, and what is made of
// its file, in what order, and taken away.
//
// A stack is admitted with its file read by the platform (stackKind.Parse),
// everything wrong with it refused under its path, compose.services.web.build
// say, and its graph planned, so that services that depend on one another in
// a ring are refused too, each of them. It goes into the Docker VM it names,
// which has to be its owner's and running or on its way up, or, when it names
// none, into one its graph is to make for it: given a uuid now, and held now
// to what the vm kind would admit it as, but made by its graph. Either way it
// keeps the VM's uuid and what to make the VM with again whenever it is gone,
// and it belongs to that VM. It is given a slug nothing else has, which what
// its graph makes is named under, and it starts deploying.
//
// A stack is the root of a graph of its own (Graph), which makes its VM, its
// networks and its volumes, and then its services' containers, as they depend
// on one another, and then it; makes each again whenever it is gone; carries
// out its start, its stop and its restart; and removes it, and what it owns,
// once it is asked to go. Its VM's disk restored from a snapshot, it is kept
// while the restored disk holds any of its containers, and forgotten, with
// what it owns, once the disk is looked into and found to hold none of them,
// nothing made of it in the meantime (Graph.Restored). Nothing else is asked
// of it: its record is the control plane's own, and no node keeps any of it.
package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// maxNameLength keeps a name to something a listing can show.
const maxNameLength = 100

// Dependencies are what the strategy chooses VMs, gives slugs and plans
// stacks' graphs with.
type Dependencies struct {
	// Chooser chooses the Docker VM a stack names, or plans the one its graph
	// is to make for it when it names none.
	Chooser *dockervm.Chooser

	// Slugs are what says which slugs are held already.
	Slugs []slugs.Taken

	// Planner plans a stack's graph, as the control plane's graphs are
	// planned: one with services that depend on one another in a ring has
	// none.
	Planner *graph.Planner
}

// Stacks is the stack kind's control-plane strategy.
type Stacks struct {
	chooser *dockervm.Chooser
	taken   []slugs.Taken
	planner *graph.Planner
}

var (
	_ kind.ControlPlane[stackKind.Spec, stackKind.Status] = &Stacks{}
	_ kind.Outliver[stackKind.Spec, stackKind.Status]     = &Stacks{}
)

// New is the strategy over what dependencies say.
func New(dependencies Dependencies) *Stacks {
	return &Stacks{chooser: dependencies.Chooser, taken: dependencies.Slugs, planner: dependencies.Planner}
}

// Admit takes in a stack its owner asked for: its name and its file checked,
// its graph planned, in the Docker VM it names, which has to be theirs and
// running or on its way up, or in one its graph is to make for it when it
// names none, held now to what the vm kind would admit it as, and given a
// slug. What it keeps of its VM is the VM's uuid and what to make it with
// again whenever it is gone, as the VM's record has it, or as it was asked
// for with the defaults filled in.
func (s *Stacks) Admit(ctx context.Context, asked stackKind.Stack) (stackKind.Stack, domain.ValidationErrors, error) {
	choice := asked.Spec.VM
	if parent, named := asked.Metadata.Owner(stackKind.Parent); named && len(choice.UUID) == 0 && !choice.Describes() {
		choice.UUID = parent.UUID
	}

	invalid := validate(asked, choice)

	file, unread := stackKind.Parse(asked.Spec.Compose)
	maps.Copy(invalid, unread)

	if len(invalid) > 0 {
		return stackKind.Stack{}, invalid, nil
	}

	v, there, refused, err := s.vmOf(ctx, asked.Metadata.OwnerUUID, choice)
	if err != nil || len(refused) > 0 {
		return stackKind.Stack{}, refused, err
	}

	name := strings.TrimSpace(asked.Metadata.Name)

	slug, err := slugs.Generate(ctx, name, s.taken...)
	if err != nil {
		return stackKind.Stack{}, nil, err
	}

	admitted := stackKind.Stack{
		Kind: stackKind.Name,
		Metadata: kind.Metadata{
			Name:      name,
			Slug:      slug,
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    asked.Metadata.Labels,
			Owners:    slices.Clone(asked.Metadata.Owners),
			Refs:      kind.LivingIn(asked.Metadata.Refs, kind.Reference{Kind: stackKind.Parent, UUID: v.Metadata.UUID}),
		},
		Spec: stackKind.Spec{
			VM:      vmKind.ChoiceOf(v),
			Compose: asked.Spec.Compose,
		},
		Status: stackKind.Status{Status: kind.Status{State: stackKind.Deploying, Expected: stackKind.Running}},
	}

	switch {
	case !there:
		admitted.Status.Reason = "its vm is being made"
	case v.Status.State != vmKind.Running:
		admitted.Status.Reason = fmt.Sprintf("its vm is %s", v.Status.State)
	}

	if invalid, err := s.planned(admitted, file); err != nil || len(invalid) > 0 {
		return stackKind.Stack{}, invalid, err
	}

	return admitted, nil, nil
}

// vmOf is the Docker VM a stack of ownerUUID's goes into, as choice says, and
// whether it is there: the one choice names, which has to be theirs and
// running or on its way up; or, when it names none, one its graph is to make
// for it, as choice describes it, planned now under a uuid of its own
// (dockervm.Chooser.Plan), which is not there yet.
func (s *Stacks) vmOf(ctx context.Context, ownerUUID string, choice stackKind.VMChoice) (vmKind.VM, bool, domain.ValidationErrors, error) {
	if len(choice.UUID) == 0 {
		v, refused, err := s.chooser.Plan(ctx, ownerUUID, choiceOf(choice))

		return v, false, refused, err
	}

	v, refused, err := s.chooser.Choose(ctx, ownerUUID, dockervm.Choice{UUID: choice.UUID})
	switch {
	case err != nil || len(refused) > 0:
		return vmKind.VM{}, false, refused, err
	case !vmKind.Up(v):
		return vmKind.VM{}, false, domain.ValidationErrors{"vm": "vm_not_running"}, nil
	}

	return v, true, nil, nil
}

// planned is what is wrong with the graph of a stack admitted with file, as
// it would be planned once it is kept: services that depend on one another
// in a ring, each refused under its depends_on, which no order starts.
// Anything else planning fails with is an error.
func (s *Stacks) planned(admitted stackKind.Stack, file stackKind.File) (domain.ValidationErrors, error) {
	// a uuid is given it as it is kept, and nothing of what it needs says
	// which: it is planned under its slug.
	w := stackKind.Want{UUID: admitted.Metadata.Slug, OwnerUUID: admitted.Metadata.OwnerUUID, Slug: admitted.Metadata.Slug, VM: admitted.Spec.VM, File: file}

	_, err := s.planner.Plan(w.Requirement())

	var cycle *graph.Cycle

	switch {
	case errors.As(err, &cycle):
		invalid := make(domain.ValidationErrors, len(cycle.Nodes))
		for _, service := range ServicesOf(w, cycle) {
			invalid[stackKind.Field("services."+service+".depends_on")] = stackKind.CodeDependsOnCycle
		}

		return invalid, nil
	case err != nil:
		return nil, fmt.Errorf("the graph of the stack cannot be planned: %w", err)
	}

	return nil, nil
}

// ServicesOf are the services of the stack w names whose containers are in a
// ring of its plan, in the ring's order: those that depend on one another,
// round to the first.
func ServicesOf(w stackKind.Want, cycle *graph.Cycle) []string {
	services := make([]string, 0, len(cycle.Nodes))

	for _, id := range cycle.Nodes {
		if service, ours := strings.CutPrefix(id.Name(), w.Named("")); ours {
			services = append(services, service)
		}
	}

	return services
}

// Reconcile asks for nothing: what a stack needs, and what it was asked, its
// graph carries out.
func (s *Stacks) Reconcile(context.Context, stackKind.Stack) ([]kind.Intent, error) {
	return nil, nil
}

// Apply takes an action asked of a stack, which its machine moved it for and
// its graph carries out: what its plan failed with before says nothing of it,
// and is let go of. An action a stack does not have is refused.
func (s *Stacks) Apply(_ context.Context, st stackKind.Stack, action string, _ any) (stackKind.Stack, domain.ValidationErrors, error) {
	switch action {
	case stackKind.ActionStart, stackKind.ActionStop, stackKind.ActionRestart, stackKind.ActionDelete:
		st.Status.Failure = nil

		return st, nil, nil
	}

	return stackKind.Stack{}, nil, fmt.Errorf("%w: a stack has no %q", kind.ErrUnknownAction, action)
}

// Outlive is a stack kept as gone with its Docker VM, whose graph makes the
// VM again under its uuid: what it keeps of its VM, and makes it with again,
// is the VM as its record had it as it went (vmKind.ChoiceOf), the name,
// resources, ports and network an update or a reconfigure gave it since it
// was named, and the slug it was reached under, which it asks for again. A
// parent that is no VM, or not its VM, says nothing of it.
func (s *Stacks) Outlive(st stackKind.Stack, parent kind.Raw) (stackKind.Stack, error) {
	if parent.Kind != stackKind.Parent || parent.Metadata.UUID != stackKind.VMOf(st) {
		return st, nil
	}

	v, err := kind.Decode[vmKind.Spec, vmKind.Status](parent)
	if err != nil {
		return stackKind.Stack{}, err
	}

	st.Spec.VM = vmKind.ChoiceOf(v)

	return st, nil
}

// validate is what is wrong with a stack as it was asked for, but its file.
func validate(asked stackKind.Stack, choice stackKind.VMChoice) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	switch name := strings.TrimSpace(asked.Metadata.Name); {
	case len(name) == 0:
		invalid["name"] = "required_field"
	case len(name) > maxNameLength:
		invalid["name"] = "invalid_name"
	}

	if len(choice.UUID) > 0 && choice.Describes() {
		invalid["vm"] = "vm_or_new_vm"
	}

	return invalid
}

// choiceOf is a stack's choice of VM as the chooser reads one.
func choiceOf(choice stackKind.VMChoice) dockervm.Choice {
	chosen := dockervm.Choice{UUID: choice.UUID, Name: choice.Name, Ports: choice.Ports}

	if choice.Resources != nil {
		chosen.Resources = &vmKind.Resources{CPUs: choice.Resources.CPUs, Memory: choice.Resources.Memory, Disk: choice.Resources.Disk}
	}

	if choice.Network != nil {
		chosen.Network = &vmKind.Network{Ingress: vm.Access(choice.Network.Ingress), Egress: vm.Access(choice.Network.Egress)}
	}

	return chosen
}
