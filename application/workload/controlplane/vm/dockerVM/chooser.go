// Package dockerVM chooses the Docker VM a container or a stack goes into.
//
// One that names a VM goes into it, and it has to be a Docker VM of the
// person asking. One that names none goes into their only Docker VM, or into
// one made for it with the defaults when they have none; with several to
// choose from, the person asking has to say which. Detection is only by kind:
// nothing ever looks inside a VM for a dockerd.
package dockerVM

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// defaultName is what a Docker VM made for a container or a stack is called
// when nobody named it.
const defaultName = "docker"

// Choice is which Docker VM to use, as a request names it.
type Choice struct {
	// UUID names a Docker VM of the person asking.
	UUID string `json:"uuid,omitempty"`

	// New makes a Docker VM with these, the defaults filling in what they
	// leave out.
	New *New `json:"new,omitempty"`
}

// New is what a Docker VM made for a container or a stack is given beyond the
// defaults.
type New struct {
	Name      string              `json:"name,omitempty"`
	Resources *createVM.Resources `json:"resources,omitempty"`
	Ports     []port.Port         `json:"ports,omitempty"`
	Network   *createVM.Network   `json:"network,omitempty"`
}

// Defaults are what a Docker VM made for a container or a stack is given.
type Defaults struct {
	Resources      vm.Resources
	Ports          []port.Port
	Network        vm.Network
	PersistentDisk bool
	Lifetime       time.Duration
}

// Chosen is the Docker VM a container or a stack goes into, and whether it was
// made for it.
type Chosen struct {
	VM      vm.VM
	Created bool
}

// Chooser chooses Docker VMs.
type Chooser struct {
	vms       vm.Repository
	createVM  *createVM.UseCase
	lifecycle *lifecycle.Lifecycle
	defaults  Defaults
}

func NewChooser(vms vm.Repository, createVM *createVM.UseCase, lifecycle *lifecycle.Lifecycle, defaults Defaults) *Chooser {
	return &Chooser{vms: vms, createVM: createVM, lifecycle: lifecycle, defaults: defaults}
}

// Choose is the Docker VM ownerUUID's choice names, or the one that is theirs to
// use, or one made for them; or why there is none, under the request's vm
// field.
func (c *Chooser) Choose(ctx context.Context, ownerUUID string, choice Choice) (Chosen, domain.ValidationErrors, error) {
	if len(ownerUUID) == 0 {
		return Chosen{}, domain.ValidationErrors{"owner_uuid": "required_field"}, nil
	}

	if len(choice.UUID) > 0 {
		return c.named(ctx, ownerUUID, choice.UUID)
	}

	if choice.New != nil {
		return c.made(ctx, ownerUUID, choice.New)
	}

	owned, err := c.vms.GetAllByOwnerAndKind(ctx, ownerUUID, vm.KindDocker)
	if err != nil {
		return Chosen{}, nil, err
	}

	usable := make([]vm.VM, 0, len(owned))
	for i := range owned {
		if owned[i].CurrentState != vm.Deleting {
			usable = append(usable, owned[i])
		}
	}

	switch len(usable) {
	case 0:
		return c.made(ctx, ownerUUID, &New{})
	case 1:
		return Chosen{VM: usable[0]}, nil, nil
	default:
		return Chosen{}, domain.ValidationErrors{"vm": "vm_required"}, nil
	}
}

// named is the Docker VM a request names, which has to be the person's own.
func (c *Chooser) named(ctx context.Context, ownerUUID string, uuid string) (Chosen, domain.ValidationErrors, error) {
	v, err := c.vms.GetOneByOwner(ctx, ownerUUID, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return Chosen{}, domain.ValidationErrors{"vm.uuid": "not_found"}, nil
	} else if err != nil {
		return Chosen{}, nil, err
	}

	switch {
	case v.Kind != vm.KindDocker:
		return Chosen{}, domain.ValidationErrors{"vm.uuid": "not_docker"}, nil
	case v.CurrentState == vm.Deleting:
		return Chosen{}, domain.ValidationErrors{"vm.uuid": "not_found"}, nil
	}

	return Chosen{VM: v}, nil, nil
}

// made is a Docker VM made for a container or a stack. Whatever it is refused
// for is reported under vm.new, where it was asked for. One no node has room
// for is not kept: nobody asked for it but the container or the stack that
// cannot go into it.
func (c *Chooser) made(ctx context.Context, ownerUUID string, asked *New) (Chosen, domain.ValidationErrors, error) {
	request := &createVM.Request{
		OwnerUUID:       ownerUUID,
		Name:            asked.Name,
		Kind:            vm.KindDocker,
		Resources:       createVM.Resources{CPUs: c.defaults.Resources.CPUs, Memory: c.defaults.Resources.Memory, Disk: c.defaults.Resources.Disk},
		Ports:           c.defaults.Ports,
		Network:         createVM.Network{Ingress: c.defaults.Network.Ingress, Egress: c.defaults.Network.Egress},
		PersistentDisk:  c.defaults.PersistentDisk,
		LifetimeSeconds: int64(c.defaults.Lifetime / time.Second),
	}

	if len(request.Name) == 0 {
		request.Name = defaultName
	}

	if asked.Resources != nil {
		request.Resources = *asked.Resources
	}

	if asked.Ports != nil {
		request.Ports = asked.Ports
	}

	if asked.Network != nil {
		if len(asked.Network.Ingress) > 0 {
			request.Network.Ingress = asked.Network.Ingress
		}

		if len(asked.Network.Egress) > 0 {
			request.Network.Egress = asked.Network.Egress
		}
	}

	response, err := c.createVM.Execute(ctx, request)
	if err != nil {
		return Chosen{}, nil, err
	}

	if len(response.ValidationErrors) > 0 {
		refused := make(domain.ValidationErrors, len(response.ValidationErrors))
		for field, code := range response.ValidationErrors {
			refused["vm.new."+field] = code
		}

		return Chosen{}, refused, nil
	}

	v, err := c.vms.GetOne(ctx, response.VM.UUID)
	if err != nil {
		return Chosen{}, nil, err
	}

	if v.CurrentState == vm.Failed && v.Reason == lifecycle.ReasonNoCapacity {
		if err := c.lifecycle.Forget(ctx, v.UUID); err != nil {
			return Chosen{}, nil, err
		}

		return Chosen{}, domain.ValidationErrors{"vm": lifecycle.ReasonNoCapacity}, nil
	}

	return Chosen{VM: v, Created: true}, nil, nil
}
