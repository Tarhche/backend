package volume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// tries is how many times a volume a graph needs is looked at and asked for
// again, when something else wrote its record in the meantime.
const tries = 3

// Graph is the volume kind as a graph needs it (graph.Kind): a volume in one
// of the person's Docker VMs, known by its Docker name in it. It needs its
// VM, wanted as it is wanted.
//
// It is made once its node has said docker has it, as its state says, and it
// is not on its way out:
//
//	state                                  made
//	present                                yes
//	failed, after docker had it            yes: as it was last seen, its own loop's to bring back
//	failed, before docker ever had it      no: what was to make it failed
//	pending, creating, missing, waiting    no
//	removing, or expected deleted          no: it is on its way out
//
// Whether docker ever had it is whether its node ever said what docker has of
// it (Status.Docker), which its record keeps once it is said.
//
// One that is not made is made: admitted when there is no record of it, its
// create asked again when its record is pending or missing, which makes one
// that went missing again, empty, and waited on while it is being made. One
// that failed before it ever was is asked for nothing until its VM is seen
// without it, when it is missing and its own loop makes it, so it is not made,
// saying why. Nothing is asked while its VM does not run, nor of one on its
// way out: both wait.
type Graph struct {
	volumes *Volumes
	records *requirements.Records
}

var _ graph.Kind = &Graph{}

// NewGraph is the volume kind as a graph needs it, over what its strategy
// reads, keeping and removing volumes through records.
func NewGraph(volumes *Volumes, records *requirements.Records) *Graph {
	return &Graph{volumes: volumes, records: records}
}

// Requires is the Docker VM a volume is made in, wanted as it is wanted, so
// that whichever requirement of it a graph keeps can make it.
func (g *Graph) Requires(spec any) ([]graph.Requirement, error) {
	w, err := wantOf(spec)
	if err != nil {
		return nil, err
	}

	return []graph.Requirement{vmKind.Want{OwnerUUID: w.OwnerUUID, VM: w.VM}.Requirement()}, nil
}

// Exists is the record of the volume the want names, whenever there is one,
// and whether its node has made it and it is not on its way out.
func (g *Graph) Exists(ctx context.Context, spec any) (graph.Ref, bool, error) {
	w, err := wantOf(spec)
	if err != nil {
		return graph.Ref{}, false, err
	}

	r, found, err := g.volumes.Recorded(ctx, w.VM.UUID, w.Spec.Name)
	if err != nil || !found {
		return graph.Ref{}, false, err
	}

	made, err := madeBy(r)
	if err != nil {
		return graph.Ref{}, false, err
	}

	return requirements.RefOf(r), made, nil
}

// Create makes the volume the want names in the Docker VM the graph made for
// it, or the want names, and is it once its node has made it.
func (g *Graph) Create(ctx context.Context, spec any, deps map[graph.NodeID]graph.Ref) (graph.Ref, error) {
	w, err := wantOf(spec)
	if err != nil {
		return graph.Ref{}, err
	}

	vmUUID := blocks.ParentOf(w.VM, deps)

	for try := 1; ; try++ {
		made, err := g.create(ctx, w, vmUUID)
		if errors.Is(err, resource.ErrConflict) && try < tries {
			continue
		}

		return made, err
	}
}

// create makes the volume w names in the Docker VM vmUUID names, once.
func (g *Graph) create(ctx context.Context, w volumeKind.Want, vmUUID string) (graph.Ref, error) {
	if err := g.volumes.Ready(ctx, vmUUID); errors.Is(err, domain.ErrNotExists) {
		return graph.Ref{}, fmt.Errorf("its vm %s is not there: %w", vmUUID, err)
	} else if err != nil {
		return graph.Ref{}, err
	}

	r, found, err := g.volumes.Recorded(ctx, vmUUID, w.Spec.Name)
	if err != nil {
		return graph.Ref{}, err
	}

	if found {
		r, err = g.again(ctx, r)
	} else {
		r, err = g.admit(ctx, w, vmUUID)
	}

	if err != nil {
		return graph.Ref{}, err
	}

	made, err := madeBy(r)
	switch {
	case err != nil:
		return graph.Ref{}, err
	case !made:
		return graph.Ref{}, requirements.NotDone(r, "made")
	}

	return requirements.RefOf(r), nil
}

// again is a volume whose record there is, made: as it is when it is made
// already, its create asked again when it is pending or missing, and waited
// on while it is being made. One on its way out, and one last seen waiting on
// its VM, which nothing can be asked of until its VM is looked into again,
// wait.
func (g *Graph) again(ctx context.Context, r resource.Record) (resource.Record, error) {
	v, err := kind.Decode[volumeKind.Spec, volumeKind.Status](r.Raw)
	if err != nil {
		return resource.Record{}, err
	}

	made, err := madeBy(r)
	if err != nil {
		return resource.Record{}, err
	}

	switch state := v.Status.Status.State; {
	case requirements.Going(v.Status.Status):
		return resource.Record{}, &graph.Waiting{Reason: "it is being removed"}
	case made:
		return r, nil
	case r.Pending != nil:
		settled, went, err := g.records.Settle(ctx, r)
		if err == nil && went {
			err = requirements.Gone(r)
		}

		return settled, err
	case state == volumeKind.Pending, state == volumeKind.Missing:
		return g.records.Ask(ctx, r, volumeKind.ActionCreate)
	case state == volumeKind.Waiting:
		return resource.Record{}, &graph.Waiting{Reason: "it was last seen waiting on its vm, which is looked into again"}
	}

	return r, nil
}

// admit admits the volume w names into the Docker VM vmUUID names, and is its
// record once its node has made it.
func (g *Graph) admit(ctx context.Context, w volumeKind.Want, vmUUID string) (resource.Record, error) {
	encoded, err := json.Marshal(w.Spec)
	if err != nil {
		return resource.Record{}, err
	}

	return g.records.Admit(ctx, admitResource.Request{
		Kind:      volumeKind.Name,
		OwnerUUID: w.OwnerUUID,
		Parent:    vmUUID,
		Manifest: kind.Raw{
			Kind:     volumeKind.Name,
			Metadata: kind.Metadata{Name: w.Spec.Name, Labels: w.Labels},
			Spec:     encoded,
		},
	})
}

// Delete removes the volume ref names, by force, and what was in it with it,
// and is done once its node has: one gone already is removed. One a container
// still mounts is not, as docker would not remove it, whatever the force.
// Nothing is asked while its VM does not run, which it waits on.
func (g *Graph) Delete(ctx context.Context, ref graph.Ref) error {
	if ref.Kind != volumeKind.Name {
		return fmt.Errorf("%w: a %s is not a volume", kind.ErrUnknownKind, ref.Kind)
	}

	r, there, err := g.records.Read(ctx, ref.Kind, ref.UUID)
	if err != nil || !there {
		return err
	}

	v, err := kind.Decode[volumeKind.Spec, volumeKind.Status](r.Raw)
	if err != nil {
		return err
	}

	// one in a VM that is not there any more goes with it.
	if err := g.volumes.Ready(ctx, volumeKind.VMOf(v)); err != nil && !errors.Is(err, domain.ErrNotExists) {
		return err
	}

	forced, err := json.Marshal(volumeKind.DeletePayload{Force: true})
	if err != nil {
		return err
	}

	return g.records.Remove(ctx, ref, forced)
}

// madeBy reports whether a volume's node has made it, as its record says
// (Graph).
func madeBy(r resource.Record) (bool, error) {
	v, err := kind.Decode[volumeKind.Spec, volumeKind.Status](r.Raw)
	if err != nil {
		return false, err
	}

	if requirements.Going(v.Status.Status) {
		return false, nil
	}

	switch v.Status.Status.State {
	case volumeKind.Present:
		return true, nil
	case volumeKind.Failed:
		return v.Status.Docker != nil, nil
	}

	return false, nil
}

// wantOf is spec as a volume a graph needs, which is named by its Docker name
// in a Docker VM named by its uuid.
func wantOf(spec any) (volumeKind.Want, error) {
	w, err := requirements.Want[volumeKind.Want](volumeKind.Name, spec)
	if err != nil {
		return volumeKind.Want{}, err
	}

	// as its admission keeps it.
	w.Spec.Name = strings.TrimSpace(w.Spec.Name)

	switch {
	case len(w.VM.UUID) == 0:
		return volumeKind.Want{}, fmt.Errorf("%w: a volume is required in a vm named by its uuid", kind.ErrInvalidPayload)
	case len(w.Spec.Name) == 0:
		return volumeKind.Want{}, fmt.Errorf("%w: a volume is required by its name", kind.ErrInvalidPayload)
	}

	return w, nil
}
