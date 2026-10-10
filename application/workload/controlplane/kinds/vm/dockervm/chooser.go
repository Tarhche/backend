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
// create at once. One a graph needs is made the same way, under the uuid what
// needs it names it by (Chooser.Make). What goes into one that a graph is to
// make, rather than the request, is given a uuid for it when it is asked for,
// and the VM is held then to what its admission would hold it to, counting
// the others planned for its owner, but not kept (Chooser.Plan): a container
// that names none.
package dockervm

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
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

// Checker holds a VM to what the vm kind would admit it as, counting what is
// planned beside its owner's VMs, and keeps nothing: the vm kind's
// control-plane strategy.
type Checker interface {
	Check(ctx context.Context, asked vmKind.VM, planned ...vmKind.Resources) (vmKind.VM, domain.ValidationErrors, error)
}

// Chooser chooses Docker VMs.
type Chooser struct {
	records   *records.Records
	resources resource.Repository
	admit     Admitter
	check     Checker
	defaults  Defaults
}

// NewChooser is a chooser that reads VMs from vms and makes them through
// admit, taking away from resources one made for nothing, and holds one a
// graph is to make to what check says of it.
func NewChooser(vms *records.Records, resources resource.Repository, admit Admitter, check Checker, defaults Defaults) *Chooser {
	return &Chooser{records: vms, resources: resources, admit: admit, check: check, defaults: defaults}
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

	return c.made(ctx, ownerUUID, "", choice)
}

// Make is a Docker VM made for ownerUUID under uuid, as one made for what
// names none is, with whatever choice gives and the defaults for the rest:
// what a graph makes for the stacks and containers that go into it, under the
// uuid they name it by, before it is there and again once it is gone. Which VM
// choice names, if it names one, is not read. Why it is not made is said
// under the request's vm field, a uuid another resource is kept under
// already being domain.ErrAlreadyExists.
func (c *Chooser) Make(ctx context.Context, ownerUUID string, uuid string, choice Choice) (vmKind.VM, domain.ValidationErrors, error) {
	switch {
	case len(ownerUUID) == 0:
		return vmKind.VM{}, domain.ValidationErrors{"owner_uuid": "required_field"}, nil
	case len(uuid) == 0:
		return vmKind.VM{}, domain.ValidationErrors{"vm.uuid": "required_field"}, nil
	}

	return c.made(ctx, ownerUUID, uuid, choice)
}

// Plan is a Docker VM a graph is to make for ownerUUID, later, for what goes
// into it and names none: under a uuid made now, with whatever choice gives
// and the defaults for the rest, as Make makes one, and held now to what the
// vm kind would admit it as, its bounds and its owner's quota, counting the
// Docker VMs graphs are to make for them and have not. Nothing is kept, and no
// node's room is looked at: one no node has room for is made once one has.
// Which VM choice names, if it names one, is not read. Why it is refused is
// said under the request's vm field, as for one made now.
func (c *Chooser) Plan(ctx context.Context, ownerUUID string, choice Choice) (vmKind.VM, domain.ValidationErrors, error) {
	if len(ownerUUID) == 0 {
		return vmKind.VM{}, domain.ValidationErrors{"owner_uuid": "required_field"}, nil
	}

	planned, err := c.planned(ctx, ownerUUID)
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	checked, refused, err := c.check.Check(ctx, c.asked(ownerUUID, choice), planned...)
	switch {
	case err != nil:
		return vmKind.VM{}, nil, err
	case len(refused) > 0:
		return vmKind.VM{}, asVM(refused), nil
	}

	id, err := uuid.NewV7()
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	checked.Metadata.UUID = id.String()

	return checked, nil, nil
}

// planned are the Docker VMs graphs are to make for ownerUUID and have not,
// each once: what each container of theirs that was not asked to go keeps of
// the VM it goes into, by a uuid none of their VMs is kept under, sized as it
// says, the defaults where it says nothing.
func (c *Chooser) planned(ctx context.Context, ownerUUID string) ([]vmKind.Resources, error) {
	owned, err := c.records.Owned(ctx, ownerUUID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(owned))
	for _, v := range owned {
		seen[v.Metadata.UUID] = true
	}

	containers, _, err := c.resources.GetAll(ctx, containerKind.Name, resource.Filter{OwnerUUID: ownerUUID}, 0, 0)
	if err != nil {
		return nil, err
	}

	var planned []vmKind.Resources

	for _, r := range containers {
		common, err := r.Common()
		if err != nil {
			return nil, err
		}

		if common.Expected == kind.Deleted || !r.Metadata.DeletionRequestedAt.IsZero() {
			continue
		}

		var spec struct {
			VM stackKind.VMChoice `json:"vm"`
		}

		if err := json.Unmarshal(r.Spec, &spec); err != nil {
			return nil, err
		}

		if len(spec.VM.UUID) == 0 || seen[spec.VM.UUID] {
			continue
		}

		seen[spec.VM.UUID] = true

		resources := c.defaults.Resources
		if asked := spec.VM.Resources; asked != nil {
			resources = sized(resources, vmKind.Resources{CPUs: asked.CPUs, Memory: asked.Memory, Disk: asked.Disk})
		}

		planned = append(planned, resources)
	}

	return planned, nil
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

// made is a Docker VM made for a container or a stack, under uuid when it is
// given one. Whatever it is refused for is reported under vm, where it was
// asked for. One no node has room for is not kept: nobody asked for it but
// the container or the stack that cannot go into it. In a dry run
// (kind.DryRun), it is admitted as a dry run too, and nothing is kept.
func (c *Chooser) made(ctx context.Context, ownerUUID string, uuid string, asked Choice) (vmKind.VM, domain.ValidationErrors, error) {
	manifest, err := kind.Encode(c.asked(ownerUUID, asked))
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	dry := kind.IsDryRun(ctx)

	admitted, err := c.admit.Execute(ctx, &admitResource.Request{Kind: vmKind.Name, OwnerUUID: ownerUUID, Manifest: manifest, UUID: uuid, DryRun: dry})
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	if len(admitted.ValidationErrors) > 0 {
		return vmKind.VM{}, asVM(admitted.ValidationErrors), nil
	}

	v, err := kind.Decode[vmKind.Spec, vmKind.Status](admitted.Resource)
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	if v.Status.State == vmKind.Failed && v.Status.Reason == ReasonNoCapacity {
		if !dry {
			if err := c.resources.Delete(ctx, vmKind.Name, v.Metadata.UUID); err != nil {
				return vmKind.VM{}, nil, err
			}
		}

		return vmKind.VM{}, domain.ValidationErrors{"vm": ReasonNoCapacity}, nil
	}

	return v, nil, nil
}

// asked is the Docker VM ownerUUID asks for when what goes into it asks for
// one as choice says: called what it is asked to be, or docker, with the
// Docker image, and the defaults with whatever it gives in place of each.
func (c *Chooser) asked(ownerUUID string, asked Choice) vmKind.VM {
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

	return vmKind.VM{
		Kind:     vmKind.Name,
		Metadata: kind.Metadata{Name: name, OwnerUUID: ownerUUID, Lifetime: c.defaults.Lifetime},
		Spec:     spec,
	}
}

// asVM is what a Docker VM was refused for, under the request's vm field,
// where it was asked for.
func asVM(refused domain.ValidationErrors) domain.ValidationErrors {
	under := make(domain.ValidationErrors, len(refused))
	for field, code := range refused {
		under["vm."+field] = code
	}

	return under
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
