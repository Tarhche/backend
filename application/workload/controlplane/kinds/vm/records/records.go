// Package records reads the VMs the control plane keeps, as the vm kind's
// manifests: by uuid, by slug, a person's own, and what one node holds.
//
// It is what the vm kind's control-plane strategy, its quota and its
// placement read VMs with; what the kinds that live in VMs read their parent
// with (Records.Down, as the observer's Parents); and, as Entities, what the
// parts of the control plane not on the framework yet read a VM with, as the
// vm package's entity: the containers, the Docker passthrough and the
// snapshots.
package records

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Records reads VMs.
type Records struct {
	resources resource.Repository
}

var _ observe.Parents = &Records{}

func New(resources resource.Repository) *Records {
	return &Records{resources: resources}
}

// GetOne is the VM uuid names, or domain.ErrNotExists.
func (r *Records) GetOne(ctx context.Context, uuid string) (vmKind.VM, error) {
	return r.decoded(r.resources.GetOne(ctx, vmKind.Name, uuid))
}

// GetOneByOwner is the VM uuid names, as one of ownerUUID's own: somebody
// else's is domain.ErrNotExists.
func (r *Records) GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (vmKind.VM, error) {
	return r.decoded(r.resources.GetOneByOwner(ctx, vmKind.Name, ownerUUID, uuid))
}

// GetOneBySlug is the VM a slug names, or domain.ErrNotExists.
func (r *Records) GetOneBySlug(ctx context.Context, slug string) (vmKind.VM, error) {
	return r.decoded(r.resources.GetOneBySlug(ctx, vmKind.Name, slug))
}

// All is every VM filter lets through, newest first.
func (r *Records) All(ctx context.Context, filter resource.Filter) ([]vmKind.VM, error) {
	records, _, err := r.resources.GetAll(ctx, vmKind.Name, filter, 0, 0)
	if err != nil {
		return nil, err
	}

	vms := make([]vmKind.VM, 0, len(records))
	for i := range records {
		v, err := Decode(records[i])
		if err != nil {
			return nil, err
		}

		vms = append(vms, v)
	}

	return vms, nil
}

// Owned is every VM ownerUUID has, newest first.
func (r *Records) Owned(ctx context.Context, ownerUUID string) ([]vmKind.VM, error) {
	if len(ownerUUID) == 0 {
		return nil, nil
	}

	return r.All(ctx, resource.Filter{OwnerUUID: ownerUUID})
}

// Held is every VM a node holds, newest first.
func (r *Records) Held(ctx context.Context, nodeName string) ([]vmKind.VM, error) {
	if len(nodeName) == 0 {
		return nil, nil
	}

	return r.All(ctx, resource.Filter{Node: nodeName})
}

// Down is the state a VM is in when it is not running, which is all that
// keeps what lives in it from being looked at. A VM that runs, one that is
// gone, and a parent that is not a VM, say nothing.
func (r *Records) Down(ctx context.Context, parent kind.Reference) (kind.State, error) {
	if parent.Kind != vmKind.Name {
		return "", nil
	}

	v, err := r.GetOne(ctx, parent.UUID)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return "", nil
	case err != nil:
		return "", err
	case v.Status.State == vmKind.Running:
		return "", nil
	}

	return v.Status.State, nil
}

// Going reports whether a VM is on its way out: being deleted, or expected
// to be, which nothing is asked of any more.
func Going(v vmKind.VM) bool {
	return v.Status.State == vmKind.Deleting || v.Status.Expected == vmKind.Deleted
}

// Decode reads a VM's record as the kind's manifest.
func Decode(record resource.Record) (vmKind.VM, error) {
	if record.Kind != vmKind.Name {
		return vmKind.VM{}, fmt.Errorf("%w: a %q is not a vm", kind.ErrUnknownKind, record.Kind)
	}

	return kind.Decode[vmKind.Spec, vmKind.Status](record.Raw)
}

func (r *Records) decoded(record resource.Record, err error) (vmKind.VM, error) {
	if err != nil {
		return vmKind.VM{}, err
	}

	return Decode(record)
}

// Entities are the VMs the control plane keeps as the vm package's entity,
// for the parts of the control plane not on the framework yet: the
// containers in Docker VMs, the Docker passthrough and the snapshots. What a
// VM's node last said of it is when it was last observed.
type Entities struct {
	records *Records
}

func NewEntities(records *Records) *Entities {
	return &Entities{records: records}
}

// GetOne is the VM uuid names, or domain.ErrNotExists.
func (e *Entities) GetOne(ctx context.Context, uuid string) (vm.VM, error) {
	v, err := e.records.GetOne(ctx, uuid)
	if err != nil {
		return vm.VM{}, err
	}

	return vmKind.Entity(v), nil
}

// GetOneByOwner is the VM uuid names, as one of ownerUUID's own.
func (e *Entities) GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (vm.VM, error) {
	v, err := e.records.GetOneByOwner(ctx, ownerUUID, uuid)
	if err != nil {
		return vm.VM{}, err
	}

	return vmKind.Entity(v), nil
}

// GetAllByOwnerAndKind is every VM of one flavor ownerUUID has, newest
// first: the Docker VMs a container can be put in.
func (e *Entities) GetAllByOwnerAndKind(ctx context.Context, ownerUUID string, flavor vm.Kind) ([]vm.VM, error) {
	vms, err := e.records.All(ctx, resource.Filter{OwnerUUID: ownerUUID, Labels: map[string]string{vmKind.LabelFlavor: string(flavor)}})
	if err != nil {
		return nil, err
	}

	return entities(vms), nil
}

// GetAll is a page of anybody's VMs, newest first.
func (e *Entities) GetAll(ctx context.Context, offset uint, limit uint) ([]vm.VM, error) {
	records, _, err := e.records.resources.GetAll(ctx, vmKind.Name, resource.Filter{}, offset, limit)
	if err != nil {
		return nil, err
	}

	vms := make([]vmKind.VM, 0, len(records))
	for i := range records {
		v, err := Decode(records[i])
		if err != nil {
			return nil, err
		}

		vms = append(vms, v)
	}

	return entities(vms), nil
}

func entities(vms []vmKind.VM) []vm.VM {
	of := make([]vm.VM, len(vms))
	for i := range vms {
		of[i] = vmKind.Entity(vms[i])
	}

	return of
}
