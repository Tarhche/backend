// Package vm is the vm kind's ingress strategy: where a VM is reached, by
// its uuid for its terminal and by its slug for its ports.
//
// Nothing is reached but through the node holding it, so where a VM is, is
// which node holds it, as its record says: the ingress's only use of the
// database. Who may open its terminal is its node's to say, from the owner
// on the VM and the token on the request; the ingress only knows where to
// carry it.
//
// A VM lets the ingress reach the ports it exposes while its ingress is
// allowed, and none at all while it is denied: a slug whose VM lets nothing
// in names nothing the ingress serves. One that is not running is there and
// cannot be reached now.
package vm

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Records are where VMs are read from: the control plane's records of them.
type Records interface {
	GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error)
	GetOneBySlug(ctx context.Context, kindName string, slug string) (resource.Record, error)
}

// Ingress is the vm kind's ingress strategy.
type Ingress struct {
	records Records
}

var _ kind.Ingress = &Ingress{}

func New(records Records) *Ingress {
	return &Ingress{records: records}
}

// ByUUID is the node holding the VM uuid names, for its terminal. One on no
// node yet is there, and on none.
func (i *Ingress) ByUUID(ctx context.Context, uuid string) (kind.Location, error) {
	v, err := i.read(i.records.GetOne(ctx, vmKind.Name, uuid))
	if err != nil {
		return kind.Location{}, err
	}

	return locationOf(v), nil
}

// BySlug is the node holding the VM a slug names, and the ports it lets the
// ingress reach, for its ports. One that lets nothing in names nothing the
// ingress serves, and one that is not running cannot be reached now, which
// says which of its ports it would let the ingress reach.
func (i *Ingress) BySlug(ctx context.Context, slug string) (kind.Location, error) {
	v, err := i.read(i.records.GetOneBySlug(ctx, vmKind.Name, slug))
	if err != nil {
		return kind.Location{}, err
	}

	location := locationOf(v)
	if len(location.Ports) == 0 {
		return kind.Location{}, fmt.Errorf("%w: the vm %q lets nothing in", domain.ErrNotExists, slug)
	}

	if v.Status.State != vmKind.Running {
		return location, fmt.Errorf("%w: the vm is not running", kind.ErrUnreachable)
	}

	return location, nil
}

// read is a VM's record as the kind's manifest.
func (i *Ingress) read(record resource.Record, err error) (vmKind.VM, error) {
	if err != nil {
		return vmKind.VM{}, err
	}

	return kind.Decode[vmKind.Spec, vmKind.Status](record.Raw)
}

// locationOf is where a VM is reached: the node holding it, and the ports it
// lets the ingress reach, which are none while its ingress is denied.
func locationOf(v vmKind.VM) kind.Location {
	location := kind.Location{UUID: v.Metadata.UUID, Node: v.Metadata.Node}

	if v.Spec.Network.Ingress == vm.AccessAllow {
		location.Ports = append([]port.Port(nil), v.Spec.Ports...)
	}

	return location
}
