// Package stack is the stack kind's control-plane strategy: what a stack is
// admitted as, and what it is asked for when what it is doing is not what it
// was asked to be.
//
// A stack is admitted into a Docker VM chosen by the rules a container's is:
// the one it names, its owner's only one, or one made for it. It is placed
// where its VM is, belongs to its VM, and is given a slug nothing else has,
// which is its compose project. It starts waiting, and is deployed as soon as
// its VM runs.
//
// What it is asked for after that is decided from what its node last said of
// it, and from its VM:
//
//	expected  observed                      asked
//	running   waiting, its VM running       create
//	running   waiting, its VM not running   nothing: the VM's own reconcile brings it up
//	running   degraded                      apply (with the loop's backoff)
//	running   failed                        apply (with the loop's backoff)
//	running   stopped                       start
//	stopped   running, degraded or failed   stop
//
// A deploy that never answers is asked again by the reconcile loop, as any
// command is, and a stack expected deleted is the loop's to delete.
package stack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// MaxCompose is the most YAML a stack may be given, in bytes.
	MaxCompose = 256 << 10

	// maxNameLength keeps a name to something a listing can show.
	maxNameLength = 100
)

// Stacks is the stack kind's control-plane strategy.
type Stacks struct {
	vms     *records.Records
	chooser *dockervm.Chooser
	taken   []slugs.Taken
}

var _ kind.ControlPlane[stackKind.Spec, stackKind.Status] = &Stacks{}

// New is the strategy that chooses Docker VMs with chooser, reads them, the
// vm kind's resources, from vms, and gives stacks slugs none of taken says
// are held.
func New(vms *records.Records, chooser *dockervm.Chooser, taken ...slugs.Taken) *Stacks {
	return &Stacks{vms: vms, chooser: chooser, taken: taken}
}

// Admit takes in a stack its owner asked for: its name and its compose file
// checked, its Docker VM chosen, or made, and its slug given. Its VM has to be
// running or on its way up; one that is stopped is refused, since nothing
// could be deployed into it.
func (s *Stacks) Admit(ctx context.Context, asked stackKind.Stack) (stackKind.Stack, domain.ValidationErrors, error) {
	choice := asked.Spec.VM
	if parent, named := asked.Metadata.Owner(stackKind.Parent); named && len(choice.UUID) == 0 && choice.New == nil {
		choice.UUID = parent.UUID
	}

	if invalid := validate(asked, choice); len(invalid) > 0 {
		return stackKind.Stack{}, invalid, nil
	}

	chosen, refused, err := s.chooser.Choose(ctx, asked.Metadata.OwnerUUID, choiceOf(choice))
	if err != nil || len(refused) > 0 {
		return stackKind.Stack{}, refused, err
	}

	v := chosen.VM

	if !vmKind.Up(v) {
		return stackKind.Stack{}, domain.ValidationErrors{"vm": "vm_not_running"}, nil
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
			Owners:    []kind.Reference{{Kind: stackKind.Parent, UUID: v.Metadata.UUID}},
			Node:      v.Metadata.Node,
		},
		Spec: stackKind.Spec{
			VM:      stackKind.VMChoice{UUID: v.Metadata.UUID},
			Compose: asked.Spec.Compose,
		},
		Status: stackKind.Status{Status: kind.Status{State: stackKind.Waiting, Expected: stackKind.Running}},
	}

	// what it was made with, or nothing more than the defaults: either way,
	// that it was made for this stack.
	if chosen.Created {
		admitted.Spec.VM.New = &stackKind.NewVM{}
		if choice.New != nil {
			admitted.Spec.VM.New = choice.New
		}
	}

	if v.Status.State != vmKind.Running {
		admitted.Status.Reason = waitingOn(v)
	}

	return admitted, nil, nil
}

// Reconcile is what a stack is asked for, given what it was asked to be and
// what it was last seen doing.
func (s *Stacks) Reconcile(ctx context.Context, r stackKind.Stack) ([]kind.Intent, error) {
	switch state, expected := r.Status.State, r.Status.Expected; {
	case expected == stackKind.Running && state == stackKind.Waiting:
		running, err := s.running(ctx, stackKind.VMOf(r))
		if err != nil || !running {
			return nil, err
		}

		return []kind.Intent{{Action: stackKind.ActionCreate, Reason: "it is not in its vm, which runs"}}, nil

	case expected == stackKind.Running && state == stackKind.Degraded:
		return []kind.Intent{{Action: stackKind.ActionApply, Reason: "some of its services are not running"}}, nil

	case expected == stackKind.Running && state == stackKind.Failed:
		return []kind.Intent{{Action: stackKind.ActionApply, Reason: "its last command failed"}}, nil

	case expected == stackKind.Running && state == stackKind.Stopped:
		return []kind.Intent{{Action: stackKind.ActionStart, Reason: "it stopped while it was expected running"}}, nil

	case expected == stackKind.Stopped && (state == stackKind.Running || state == stackKind.Degraded || state == stackKind.Failed):
		return []kind.Intent{{Action: stackKind.ActionStop, Reason: "it runs while it was expected stopped"}}, nil
	}

	return nil, nil
}

// Apply refuses every action, since a stack has none that runs in the
// control plane.
func (s *Stacks) Apply(_ context.Context, _ stackKind.Stack, action string, _ any) (stackKind.Stack, domain.ValidationErrors, error) {
	return stackKind.Stack{}, nil, fmt.Errorf("%w: a stack has no %q run in the control plane", kind.ErrUnknownAction, action)
}

// running reports whether the VM uuid names is running. One that is not
// there is not, and the stacks in it go with it.
func (s *Stacks) running(ctx context.Context, uuid string) (bool, error) {
	v, err := s.vms.GetOne(ctx, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return v.Status.State == vmKind.Running, nil
}

// waitingOn is why a stack waits on a VM that is not running yet.
func waitingOn(v vmKind.VM) string {
	return fmt.Sprintf("its vm is %s", v.Status.State)
}

// validate is what is wrong with a stack as it was asked for.
func validate(asked stackKind.Stack, choice stackKind.VMChoice) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	switch name := strings.TrimSpace(asked.Metadata.Name); {
	case len(name) == 0:
		invalid["name"] = "required_field"
	case len(name) > maxNameLength:
		invalid["name"] = "invalid_name"
	}

	if code, ok := ValidateCompose(asked.Spec.Compose); !ok {
		invalid["compose"] = code
	}

	if len(choice.UUID) > 0 && choice.New != nil {
		invalid["vm"] = "vm_or_new_vm"
	}

	return invalid
}

// ValidateCompose checks that compose is YAML a compose project can be made
// of: no larger than a stack may be given, and with a service at least.
// Anything finer is compose's own to refuse, which it does when the stack is
// deployed, in words that end up in the stack's output.
func ValidateCompose(compose string) (string, bool) {
	if len(strings.TrimSpace(compose)) == 0 {
		return "required_field", false
	}

	if len(compose) > MaxCompose {
		return "too_large", false
	}

	var project struct {
		Services map[string]any `yaml:"services"`
	}

	if err := yaml.Unmarshal([]byte(compose), &project); err != nil {
		return "invalid_value", false
	}

	if len(project.Services) == 0 {
		return "invalid_value", false
	}

	return "", true
}

// choiceOf is a stack's choice of VM as the chooser reads one.
func choiceOf(choice stackKind.VMChoice) dockervm.Choice {
	chosen := dockervm.Choice{UUID: choice.UUID}

	if made := choice.New; made != nil {
		chosen.New = &dockervm.New{Name: made.Name, Ports: made.Ports}

		if made.Resources != nil {
			chosen.New.Resources = &vmKind.Resources{CPUs: made.Resources.CPUs, Memory: made.Resources.Memory, Disk: made.Resources.Disk}
		}

		if made.Network != nil {
			chosen.New.Network = &vmKind.Network{Ingress: vm.Access(made.Network.Ingress), Egress: vm.Access(made.Network.Egress)}
		}
	}

	return chosen
}
