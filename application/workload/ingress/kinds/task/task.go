// Package task is the task kind's ingress strategy: where a task is reached,
// by its uuid for its terminal and by its slug for its ports.
//
// Nothing is reached but through the node holding it, so where a task is, is
// which node holds it, as that node last said, less what a command on its
// way to it takes away (ingress.Locations): the ingress reads no records.
// What is said of a task is read here too, since this is what knows a task:
// the slug its run was made with, what it is doing and the ports its run
// publishes, as its status says, and the ports it lets in as its record says,
// as a command carries it. Who may open its terminal is its node's to say,
// from the run itself and the token on the request, if there is one: a
// snippet's terminal is anybody's. The ingress only knows where to carry it.
//
// A task lets the ingress reach the ports it serves while its network policy
// lets anything in, and none at all otherwise. One that is not running is
// there and cannot be reached now; one no node has said anything of lately is
// not there.
package task

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Ingress is the task kind's ingress strategy.
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

// ByUUID is the node holding the task uuid names, for its terminal.
func (i *Ingress) ByUUID(ctx context.Context, uuid string) (kind.Location, error) {
	heard, err := i.locations.ByUUID(ctx, taskKind.Name, uuid)
	if err != nil {
		return kind.Location{}, err
	}

	return locationOf(heard), nil
}

// BySlug is the node holding the task a slug names, and the ports it lets
// the ingress reach, for its ports. One that is not running cannot be reached
// now, which says which of its ports it would let the ingress reach.
func (i *Ingress) BySlug(ctx context.Context, slug string) (kind.Location, error) {
	heard, err := i.locations.BySlug(ctx, taskKind.Name, slug)
	if err != nil {
		return kind.Location{}, err
	}

	location := locationOf(heard)

	if heard.State != taskKind.Running {
		return location, fmt.Errorf("%w: the task is not running", kind.ErrUnreachable)
	}

	return location, nil
}

// Descriptor is the task kind.
func (i *Ingress) Descriptor() kind.Descriptor {
	return taskKind.Descriptor()
}

// Running is the state a task is reached in.
func (i *Ingress) Running() kind.State {
	return taskKind.Running
}

// Read is where a task is, as its node says: the slug its run was made
// with, what it is doing, and the ports its run publishes, whether or not
// they are up now. A task no node has run is reached under no slug, and on
// no port.
func (i *Ingress) Read(status json.RawMessage) (ingress.Heard, error) {
	var said taskKind.Status
	if err := json.Unmarshal(status, &said); err != nil {
		return ingress.Heard{}, fmt.Errorf("the task's status cannot be read: %w", err)
	}

	heard := ingress.Heard{State: said.State}

	if said.Run != nil {
		heard.Slug = said.Run.Slug
		heard.Ports = slices.Clone(said.Run.Ports)
	}

	return heard, nil
}

// Allowed are the ports a task lets the ingress reach as the control plane
// recorded it: the ports it serves, while its network policy lets anything
// in, and none otherwise.
func (i *Ingress) Allowed(r kind.Raw) ([]port.Port, error) {
	t, err := kind.Decode[taskKind.Spec, taskKind.Status](r)
	if err != nil {
		return nil, err
	}

	if !t.Spec.Policy().AllowsPorts() {
		return nil, nil
	}

	return slices.Clone(t.Spec.Ports), nil
}

// locationOf is where a task is reached: the node holding it, and the ports
// it lets the ingress reach, which are none under a policy that lets nothing
// in.
func locationOf(heard ingress.Heard) kind.Location {
	return kind.Location{UUID: heard.UUID, Node: heard.Node, Ports: slices.Clone(heard.Ports)}
}
