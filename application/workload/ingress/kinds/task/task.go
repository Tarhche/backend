// Package task is the task kind's ingress strategy: where a task is reached,
// by its uuid for its terminal and by its slug for its ports.
//
// Nothing is reached but through the node holding it, so where a task is, is
// which node holds it, as its record says: the ingress's only use of the
// database. Who may open its terminal is its node's to say, from the run
// itself and the token on the request, if there is one: a snippet's terminal
// is anybody's. The ingress only knows where to carry it.
//
// A task lets the ingress reach the ports it serves while its network policy
// lets anything in, and none at all otherwise. One that is not running is
// there and cannot be reached now.
package task

import (
	"context"
	"fmt"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Records are where tasks are read from: the control plane's records of them.
type Records interface {
	GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error)
	GetOneBySlug(ctx context.Context, kindName string, slug string) (resource.Record, error)
}

// Ingress is the task kind's ingress strategy.
type Ingress struct {
	records Records
}

var _ kind.Ingress = &Ingress{}

func New(records Records) *Ingress {
	return &Ingress{records: records}
}

// ByUUID is the node holding the task uuid names, for its terminal. One on no
// node yet is there, and on none.
func (i *Ingress) ByUUID(ctx context.Context, uuid string) (kind.Location, error) {
	t, err := read(i.records.GetOne(ctx, taskKind.Name, uuid))
	if err != nil {
		return kind.Location{}, err
	}

	return locationOf(t), nil
}

// BySlug is the node holding the task a slug names, and the ports it lets
// the ingress reach, for its ports. One that is not running cannot be reached
// now, which says which of its ports it would let the ingress reach.
func (i *Ingress) BySlug(ctx context.Context, slug string) (kind.Location, error) {
	t, err := read(i.records.GetOneBySlug(ctx, taskKind.Name, slug))
	if err != nil {
		return kind.Location{}, err
	}

	location := locationOf(t)

	if t.Status.State != taskKind.Running {
		return location, fmt.Errorf("%w: the task is not running", kind.ErrUnreachable)
	}

	return location, nil
}

// read is a task's record as the kind's manifest.
func read(record resource.Record, err error) (taskKind.Task, error) {
	if err != nil {
		return taskKind.Task{}, err
	}

	return kind.Decode[taskKind.Spec, taskKind.Status](record.Raw)
}

// locationOf is where a task is reached: the node holding it, and the ports
// it lets the ingress reach, which are none under a policy that lets nothing
// in.
func locationOf(t taskKind.Task) kind.Location {
	location := kind.Location{UUID: t.Metadata.UUID, Node: t.Metadata.Node}

	if t.Spec.Policy().AllowsPorts() {
		location.Ports = slices.Clone(t.Spec.Ports)
	}

	return location
}
