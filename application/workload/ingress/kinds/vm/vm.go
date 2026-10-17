// Package vm is the vm kind's ingress strategy: where a VM is reached, by
// its uuid for its terminal and by its slug for its ports.
//
// Nothing is reached but through the node holding it, so where a VM is, is
// which node holds it, as that node last said, less what a command on its
// way to it takes away (ingress.Locations): the ingress reads no records.
// What is said of a VM is read here too, since this is what knows a VM: the
// slug it was given, what it is doing and the ports its node published for
// it, as its status says, and the ports it lets in as its record says, as a
// command carries it. Who may open its terminal is its node's to say, from
// the owner on the VM and the token on the request; the ingress only knows
// where to carry it.
//
// A VM lets the ingress reach the ports it exposes while its ingress is
// allowed, and none at all while it is denied, which is what its node
// publishes for it, whether it runs or not: as its node last applied them. A
// change to either takes away at once what it takes away, as the command to
// apply it is sent, and gives what it gives once its node has applied it. A
// slug whose VM lets nothing in names nothing the ingress serves. One that is
// not running is there and cannot be reached now; one no node has said
// anything of lately is not there.
package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Ingress is the vm kind's ingress strategy.
type Ingress struct {
	locations ingress.Locations
}

var (
	_ kind.Ingress         = &Ingress{}
	_ locateResources.Kind = &Ingress{}
)

func New(locations ingress.Locations) *Ingress {
	return &Ingress{locations: locations}
}

// ByUUID is the node holding the VM uuid names, for its terminal.
func (i *Ingress) ByUUID(ctx context.Context, uuid string) (kind.Location, error) {
	heard, err := i.locations.ByUUID(ctx, vmKind.Name, uuid)
	if err != nil {
		return kind.Location{}, err
	}

	return locationOf(heard), nil
}

// BySlug is the node holding the VM a slug names, and the ports it lets the
// ingress reach, for its ports. One that lets nothing in names nothing the
// ingress serves, and one that is not running cannot be reached now, which
// says which of its ports it would let the ingress reach.
func (i *Ingress) BySlug(ctx context.Context, slug string) (kind.Location, error) {
	heard, err := i.locations.BySlug(ctx, vmKind.Name, slug)
	if err != nil {
		return kind.Location{}, err
	}

	location := locationOf(heard)
	if len(location.Ports) == 0 {
		return kind.Location{}, fmt.Errorf("%w: the vm %q lets nothing in", domain.ErrNotExists, slug)
	}

	if heard.State != vmKind.Running {
		return location, fmt.Errorf("%w: the vm is not running", kind.ErrUnreachable)
	}

	return location, nil
}

// Descriptor is the vm kind.
func (i *Ingress) Descriptor() kind.Descriptor {
	return vmKind.Descriptor()
}

// Running is the state a VM is reached in.
func (i *Ingress) Running() kind.State {
	return vmKind.Running
}

// Read is where a VM is, as its node says: the slug it was given, what it
// is doing, and the ports of the endpoints its node published for it, which
// are the ports it lets the ingress reach.
func (i *Ingress) Read(status json.RawMessage) (ingress.Heard, error) {
	var said vmKind.Status
	if err := json.Unmarshal(status, &said); err != nil {
		return ingress.Heard{}, fmt.Errorf("the vm's status cannot be read: %w", err)
	}

	heard := ingress.Heard{Slug: said.Slug, State: said.State}

	for _, endpoint := range said.Endpoints {
		heard.Ports = append(heard.Ports, endpoint.Port)
	}

	slices.Sort(heard.Ports)
	heard.Ports = slices.Compact(heard.Ports)

	return heard, nil
}

// Allowed are the ports a VM lets the ingress reach as the control plane
// recorded it: the ports it exposes, while its ingress is allowed, and none
// while it is denied.
func (i *Ingress) Allowed(r kind.Raw) ([]port.Port, error) {
	v, err := kind.Decode[vmKind.Spec, vmKind.Status](r)
	if err != nil {
		return nil, err
	}

	if v.Spec.Network.Ingress != vm.AccessAllow {
		return nil, nil
	}

	return slices.Clone(v.Spec.Ports), nil
}

// locationOf is where a VM is reached: the node holding it, and the ports it
// lets the ingress reach, which are none while its ingress is denied.
func locationOf(heard ingress.Heard) kind.Location {
	return kind.Location{UUID: heard.UUID, Node: heard.Node, Ports: slices.Clone(heard.Ports)}
}
