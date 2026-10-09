// Package dockervm chooses the Docker VM a container or a stack goes into.
//
// One that names a VM goes into it, and it has to be a Docker VM of the
// person asking. One that names none goes into their only Docker VM, or into
// one made for it with the defaults when they have none; with several to
// choose from, the person asking has to say which. A Docker VM is one whose
// image is the Docker image, or another tag of it (vm.KindOf), and nothing
// else tells one: nothing ever looks inside a VM for a dockerd.
//
// A Docker VM made for a container or a stack is admitted as any VM is,
// through the control plane's admission of the vm kind: its bounds, its
// owner's quota and its placement are the vm kind's, and it is sent its
// create at once.
package dockervm

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// defaultName is what a Docker VM made for a container or a stack is
	// called when nobody named it.
	defaultName = "docker"

	// ReasonNoCapacity is why a VM no node had room for is not made.
	ReasonNoCapacity = "no_capacity"
)

// Choice is which Docker VM to use, as a request names it.
type Choice struct {
	// UUID names a Docker VM of the person asking.
	UUID string `json:"uuid,omitempty"`

	// New makes a Docker VM with these, the defaults filling in what they
	// leave out.
	New *New `json:"new,omitempty"`
}

// New is what a Docker VM made for a container or a stack is given beyond the
// defaults. Whatever it leaves out is the default, field by field: a size of
// zero, as a form leaves one somebody did not touch, is the default size, and
// ports or a network way that are not there are the default ones. Ports that
// are there and empty are none.
type New struct {
	Name      string            `json:"name,omitempty"`
	Resources *vmKind.Resources `json:"resources,omitempty"`
	Ports     []port.Port       `json:"ports,omitempty"`
	Network   *vmKind.Network   `json:"network,omitempty"`
}

// Defaults are what a Docker VM made for a container or a stack is given.
type Defaults struct {
	// Image is the Docker image, which a Docker VM boots from and which is
	// what makes it one: a person's Docker VMs are those of their VMs that
	// boot it.
	Image string

	Resources      vmKind.Resources
	Ports          []port.Port
	Network        vmKind.Network
	PersistentDisk bool
	Lifetime       time.Duration
}

// Chosen is the Docker VM a container or a stack goes into, and whether it
// was made for it.
type Chosen struct {
	VM      vmKind.VM
	Created bool
}

// Admitter admits a resource of any kind, as the control plane's admission
// does: a Docker VM made for a container or a stack is admitted as any VM.
type Admitter interface {
	Execute(ctx context.Context, request *admitResource.Request) (*admitResource.Response, error)
}

// Chooser chooses Docker VMs.
type Chooser struct {
	records   *records.Records
	resources resource.Repository
	admit     Admitter
	defaults  Defaults
}

// NewChooser is a chooser that reads VMs from vms and makes them through
// admit, taking away from resources one made for nothing.
func NewChooser(vms *records.Records, resources resource.Repository, admit Admitter, defaults Defaults) *Chooser {
	return &Chooser{records: vms, resources: resources, admit: admit, defaults: defaults}
}

// Choose is the Docker VM ownerUUID's choice names, or the one that is
// theirs to use, or one made for them; or why there is none, under the
// request's vm field.
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

	owned, err := c.records.Owned(ctx, ownerUUID)
	if err != nil {
		return Chosen{}, nil, err
	}

	usable := make([]vmKind.VM, 0, len(owned))
	for _, v := range owned {
		if vmKind.DockerVM(v, c.defaults.Image) && !records.Going(v) {
			usable = append(usable, v)
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
	v, err := c.records.GetOneByOwner(ctx, ownerUUID, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return Chosen{}, domain.ValidationErrors{"vm.uuid": "not_found"}, nil
	} else if err != nil {
		return Chosen{}, nil, err
	}

	switch {
	case !vmKind.DockerVM(v, c.defaults.Image):
		return Chosen{}, domain.ValidationErrors{"vm.uuid": "not_docker"}, nil
	case records.Going(v):
		return Chosen{}, domain.ValidationErrors{"vm.uuid": "not_found"}, nil
	}

	return Chosen{VM: v}, nil, nil
}

// made is a Docker VM made for a container or a stack. Whatever it is refused
// for is reported under vm.new, where it was asked for. One no node has room
// for is not kept: nobody asked for it but the container or the stack that
// cannot go into it.
func (c *Chooser) made(ctx context.Context, ownerUUID string, asked *New) (Chosen, domain.ValidationErrors, error) {
	name := asked.Name
	if len(name) == 0 {
		name = defaultName
	}

	spec := vmKind.Spec{
		Image:          c.defaults.Image,
		Resources:      c.defaults.Resources,
		Ports:          c.defaults.Ports,
		Network:        c.defaults.Network,
		PersistentDisk: c.defaults.PersistentDisk,
	}

	if asked.Resources != nil {
		spec.Resources = sized(spec.Resources, *asked.Resources)
	}

	if asked.Ports != nil {
		spec.Ports = asked.Ports
	}

	if asked.Network != nil {
		if len(asked.Network.Ingress) > 0 {
			spec.Network.Ingress = asked.Network.Ingress
		}

		if len(asked.Network.Egress) > 0 {
			spec.Network.Egress = asked.Network.Egress
		}
	}

	manifest, err := kind.Encode(vmKind.VM{
		Kind:     vmKind.Name,
		Metadata: kind.Metadata{Name: name, Lifetime: c.defaults.Lifetime},
		Spec:     spec,
	})
	if err != nil {
		return Chosen{}, nil, err
	}

	admitted, err := c.admit.Execute(ctx, &admitResource.Request{Kind: vmKind.Name, OwnerUUID: ownerUUID, Manifest: manifest})
	if err != nil {
		return Chosen{}, nil, err
	}

	if len(admitted.ValidationErrors) > 0 {
		refused := make(domain.ValidationErrors, len(admitted.ValidationErrors))
		for field, code := range admitted.ValidationErrors {
			refused["vm.new."+field] = code
		}

		return Chosen{}, refused, nil
	}

	v, err := kind.Decode[vmKind.Spec, vmKind.Status](admitted.Resource)
	if err != nil {
		return Chosen{}, nil, err
	}

	if v.Status.State == vmKind.Failed && v.Status.Reason == ReasonNoCapacity {
		if err := c.resources.Delete(ctx, vmKind.Name, v.Metadata.UUID); err != nil {
			return Chosen{}, nil, err
		}

		return Chosen{}, domain.ValidationErrors{"vm": ReasonNoCapacity}, nil
	}

	return Chosen{VM: v, Created: true}, nil, nil
}

// sized is the defaults with what was asked for in place of each, where it
// asked for anything: a size of zero is one nobody chose.
func sized(defaults vmKind.Resources, asked vmKind.Resources) vmKind.Resources {
	if asked.CPUs > 0 {
		defaults.CPUs = asked.CPUs
	}

	if asked.Memory > 0 {
		defaults.Memory = asked.Memory
	}

	if asked.Disk > 0 {
		defaults.Disk = asked.Disk
	}

	return defaults
}

// NetworkOf is a Docker VM's default network as the configuration says it,
// which has to be allow or deny each way.
func NetworkOf(ingress string, egress string) (vmKind.Network, bool) {
	network := vmKind.Network{Ingress: vm.Access(ingress), Egress: vm.Access(egress)}

	return network, network.Ingress.IsValid() && network.Egress.IsValid()
}
