// Package dockervm chooses the Docker VM a container or a stack goes into.
//
// One that names a VM goes into it, and it has to be a Docker VM of the
// person asking. One that names none goes into one made for it, with
// whatever it gives and the defaults for the rest, whichever Docker VMs the
// person has already. A Docker VM is one whose image is the Docker image, or
// another tag of it (vm.KindOf), and nothing else tells one: nothing ever
// looks inside a VM for a dockerd.
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

// Choice is which Docker VM to use, as a container's or a stack's spec names
// it: the one UUID names, or one made with the rest when it names none.
type Choice struct {
	// UUID names a Docker VM of the person asking.
	UUID string `json:"uuid,omitempty"`

	// Name, Resources, Ports and Network are what a Docker VM made for a
	// container or a stack is given beyond the defaults. Whatever they leave
	// out is the default, field by field: a size of zero, as a form leaves one
	// somebody did not touch, is the default size, and ports or a network way
	// that are not there are the default ones. Ports that are there and empty
	// are none.
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

// Choose is the Docker VM ownerUUID's choice names, or one made for them when
// it names none; or why there is none, under the request's vm field.
func (c *Chooser) Choose(ctx context.Context, ownerUUID string, choice Choice) (vmKind.VM, domain.ValidationErrors, error) {
	if len(ownerUUID) == 0 {
		return vmKind.VM{}, domain.ValidationErrors{"owner_uuid": "required_field"}, nil
	}

	if len(choice.UUID) > 0 {
		return c.named(ctx, ownerUUID, choice.UUID)
	}

	return c.made(ctx, ownerUUID, choice)
}

// named is the Docker VM a request names, which has to be the person's own.
func (c *Chooser) named(ctx context.Context, ownerUUID string, uuid string) (vmKind.VM, domain.ValidationErrors, error) {
	v, err := c.records.GetOneByOwner(ctx, ownerUUID, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return vmKind.VM{}, domain.ValidationErrors{"vm.uuid": "not_found"}, nil
	} else if err != nil {
		return vmKind.VM{}, nil, err
	}

	switch {
	case !vmKind.DockerVM(v, c.defaults.Image):
		return vmKind.VM{}, domain.ValidationErrors{"vm.uuid": "not_docker"}, nil
	case records.Going(v):
		return vmKind.VM{}, domain.ValidationErrors{"vm.uuid": "not_found"}, nil
	}

	return v, nil, nil
}

// made is a Docker VM made for a container or a stack. Whatever it is refused
// for is reported under vm, where it was asked for. One no node has room for
// is not kept: nobody asked for it but the container or the stack that cannot
// go into it.
func (c *Chooser) made(ctx context.Context, ownerUUID string, asked Choice) (vmKind.VM, domain.ValidationErrors, error) {
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
		return vmKind.VM{}, nil, err
	}

	admitted, err := c.admit.Execute(ctx, &admitResource.Request{Kind: vmKind.Name, OwnerUUID: ownerUUID, Manifest: manifest})
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	if len(admitted.ValidationErrors) > 0 {
		refused := make(domain.ValidationErrors, len(admitted.ValidationErrors))
		for field, code := range admitted.ValidationErrors {
			refused["vm."+field] = code
		}

		return vmKind.VM{}, refused, nil
	}

	v, err := kind.Decode[vmKind.Spec, vmKind.Status](admitted.Resource)
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	if v.Status.State == vmKind.Failed && v.Status.Reason == ReasonNoCapacity {
		if err := c.resources.Delete(ctx, vmKind.Name, v.Metadata.UUID); err != nil {
			return vmKind.VM{}, nil, err
		}

		return vmKind.VM{}, domain.ValidationErrors{"vm": ReasonNoCapacity}, nil
	}

	return v, nil, nil
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
