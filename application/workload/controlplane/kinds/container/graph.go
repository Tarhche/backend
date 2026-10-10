package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// tries is how many times a container a graph needs is looked at and asked
// for again, when something else wrote its record, or kept one under its
// uuid, in the meantime.
const tries = 3

// Graph is the container kind as a graph needs it (graph.Kind): a container
// in one of the person's Docker VMs, known by its record's uuid when it is a
// graph's root, one somebody asked for, and by its Docker name in its VM
// otherwise.
//
// What it needs is its Docker VM, each network it joins but those every
// dockerd has of its own, and each volume it mounts by its name; a network or
// a volume it says is not its own (Spec.External) it needs all the same, as
// what a graph never makes or removes for it.
//
// It is made once its node has said docker has it, as its state says, and it
// is not on its way out:
//
//	state                                  made
//	running, stopped, completed            yes
//	starting, stopping, restarting         yes: docker has it, and is asked something of it
//	failed, after docker had it            yes: as it was last seen, its own loop's to bring back
//	failed, before docker ever had it      no: what was to make it failed
//	pending, creating, missing, waiting    no
//	removing, or expected deleted          no: it is on its way out
//
// Whether docker ever had it is whether its node ever said what docker has of
// it (Status.Docker), which its record keeps once it is said.
//
// One that is not made is made: admitted when there is no record of it, its
// create asked again when its record is pending or missing, started, which
// makes it, when it failed before it ever was, and waited on while it is being
// made. Nothing is asked of it while its VM does not run, nor of one on its
// way out: both wait.
type Graph struct {
	containers *Containers
	records    *requirements.Records
}

var _ graph.Kind = &Graph{}

// NewGraph is the container kind as a graph needs it, over what its
// strategy reads, keeping and removing containers through records.
func NewGraph(containers *Containers, records *requirements.Records) *Graph {
	return &Graph{containers: containers, records: records}
}

// Requires is what a container needs directly: its Docker VM, wanted as it
// is wanted, so that whichever requirement of it a graph keeps can make it;
// each network it joins but those every dockerd has of its own; and each
// volume it mounts by its name. One it says is not its own is External.
func (g *Graph) Requires(spec any) ([]graph.Requirement, error) {
	w, err := wantOf(spec)
	if err != nil {
		return nil, err
	}

	required := []graph.Requirement{vmKind.Want{OwnerUUID: w.OwnerUUID, VM: w.VM}.Requirement()}

	need := func(r graph.Requirement) {
		if slices.ContainsFunc(required, func(other graph.Requirement) bool { return other.ID() == r.ID() }) {
			return
		}

		r.External = slices.Contains(w.Spec.External, r.ID().String())
		required = append(required, r)
	}

	for _, network := range w.Spec.Networks {
		if len(strings.TrimSpace(network)) == 0 || networkKind.Default(network) {
			continue
		}

		need(networkKind.Want{OwnerUUID: w.OwnerUUID, VM: w.VM, Spec: networkKind.Spec{Name: network}}.Requirement())
	}

	// a volume mounted by its name: not a path of its VM, not a tmpfs, and
	// not one docker makes for it and names itself.
	for _, m := range w.Spec.Mounts {
		if m.Type != containerKind.MountVolume || len(strings.TrimSpace(m.Source)) == 0 {
			continue
		}

		need(volumeKind.Want{OwnerUUID: w.OwnerUUID, VM: w.VM, Spec: volumeKind.Spec{Name: m.Source}}.Requirement())
	}

	return required, nil
}

// Exists is the container the want names, when its node has made it and it
// is not on its way out.
func (g *Graph) Exists(ctx context.Context, spec any) (graph.Ref, bool, error) {
	w, err := wantOf(spec)
	if err != nil {
		return graph.Ref{}, false, err
	}

	r, found, err := g.find(ctx, w, w.VM.UUID)
	if err != nil || !found {
		return graph.Ref{}, false, err
	}

	made, err := madeBy(r)
	if err != nil || !made {
		return graph.Ref{}, false, err
	}

	return requirements.RefOf(r), true, nil
}

// Create makes the container the want names in the Docker VM the graph made
// for it, or the want names, and is it once its node has made it.
func (g *Graph) Create(ctx context.Context, spec any, deps map[graph.NodeID]graph.Ref) (graph.Ref, error) {
	w, err := wantOf(spec)
	if err != nil {
		return graph.Ref{}, err
	}

	vmUUID := blocks.ParentOf(w.VM, deps)

	// what wrote its record in the meantime, or kept one under its uuid, is
	// looked at again.
	for try := 1; ; try++ {
		made, err := g.create(ctx, w, vmUUID)
		if (errors.Is(err, resource.ErrConflict) || errors.Is(err, domain.ErrAlreadyExists)) && try < tries {
			continue
		}

		return made, err
	}
}

// create makes the container w names in the Docker VM vmUUID names, once.
func (g *Graph) create(ctx context.Context, w containerKind.Want, vmUUID string) (graph.Ref, error) {
	if err := g.containers.Ready(ctx, vmUUID); errors.Is(err, domain.ErrNotExists) {
		return graph.Ref{}, fmt.Errorf("its vm %s is not there: %w", vmUUID, err)
	} else if err != nil {
		return graph.Ref{}, err
	}

	r, found, err := g.find(ctx, w, vmUUID)
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

// again is a container whose record there is, made: as it is when it is made
// already, its create asked again when it is pending or missing, started,
// which makes it, when it failed before it ever was, and waited on while it
// is being made. One on its way out, and one last seen waiting on its VM,
// which nothing can be asked of until its VM is looked into again, wait.
func (g *Graph) again(ctx context.Context, r resource.Record) (resource.Record, error) {
	c, err := kind.Decode[containerKind.Spec, containerKind.Status](r.Raw)
	if err != nil {
		return resource.Record{}, err
	}

	made, err := madeBy(r)
	if err != nil {
		return resource.Record{}, err
	}

	switch state := c.Status.Status.State; {
	case requirements.Going(c.Status.Status):
		return resource.Record{}, &graph.Waiting{Reason: "it is being removed"}
	case made:
		return r, nil
	case r.Pending != nil:
		settled, went, err := g.records.Settle(ctx, r)
		if err == nil && went {
			err = requirements.Gone(r)
		}

		return settled, err
	case state == containerKind.Pending, state == containerKind.Missing:
		return g.records.Ask(ctx, r, containerKind.ActionCreate)
	case state == containerKind.Failed:
		return g.records.Ask(ctx, r, containerKind.ActionStart)
	case state == containerKind.Waiting:
		return resource.Record{}, &graph.Waiting{Reason: "it was last seen waiting on its vm, which is looked into again"}
	}

	return r, nil
}

// admit admits the container w names into the Docker VM vmUUID names, and is
// its record once its node has made it.
func (g *Graph) admit(ctx context.Context, w containerKind.Want, vmUUID string) (resource.Record, error) {
	spec := w.Spec
	spec.VM = stackKind.VMChoice{UUID: vmUUID}

	encoded, err := json.Marshal(spec)
	if err != nil {
		return resource.Record{}, err
	}

	return g.records.Admit(ctx, admitResource.Request{
		Kind:      containerKind.Name,
		OwnerUUID: w.OwnerUUID,
		Parent:    vmUUID,
		UUID:      w.UUID,
		Manifest: kind.Raw{
			Kind:     containerKind.Name,
			Metadata: kind.Metadata{Name: spec.Name, Labels: w.Labels},
			Spec:     encoded,
		},
	})
}

// Delete removes the container ref names, running or not, by force, and is
// done once its node has: one gone already is removed. Nothing is asked while
// its VM does not run, which it waits on.
func (g *Graph) Delete(ctx context.Context, ref graph.Ref) error {
	if ref.Kind != containerKind.Name {
		return fmt.Errorf("%w: a %s is not a container", kind.ErrUnknownKind, ref.Kind)
	}

	r, there, err := g.records.Read(ctx, ref.Kind, ref.UUID)
	if err != nil || !there {
		return err
	}

	c, err := kind.Decode[containerKind.Spec, containerKind.Status](r.Raw)
	if err != nil {
		return err
	}

	// one in a VM that is not there any more goes with it.
	if err := g.containers.Ready(ctx, containerKind.VMOf(c)); err != nil && !errors.Is(err, domain.ErrNotExists) {
		return err
	}

	forced, err := json.Marshal(containerKind.DeletePayload{Force: true})
	if err != nil {
		return err
	}

	return g.records.Remove(ctx, ref, forced)
}

// find is the record of the container w names: the one its uuid names when it
// is known by one, and the one asked for in the Docker VM vmUUID names under
// its Docker name otherwise.
func (g *Graph) find(ctx context.Context, w containerKind.Want, vmUUID string) (resource.Record, bool, error) {
	if len(w.UUID) > 0 {
		return g.records.Read(ctx, containerKind.Name, w.UUID)
	}

	if len(w.Spec.Name) == 0 {
		return resource.Record{}, false, fmt.Errorf("%w: a container is required by its name or by its uuid", kind.ErrInvalidPayload)
	}

	return g.containers.Recorded(ctx, vmUUID, w.Spec.Name)
}

// madeBy reports whether a container's node has made it, as its record says
// (Graph).
func madeBy(r resource.Record) (bool, error) {
	c, err := kind.Decode[containerKind.Spec, containerKind.Status](r.Raw)
	if err != nil {
		return false, err
	}

	if requirements.Going(c.Status.Status) {
		return false, nil
	}

	switch c.Status.Status.State {
	case containerKind.Running, containerKind.Stopped, containerKind.Completed, containerKind.Starting, containerKind.Stopping, containerKind.Restarting:
		return true, nil
	case containerKind.Failed:
		return c.Status.Docker != nil, nil
	}

	return false, nil
}

// wantOf is spec as a container a graph needs, which is in a Docker VM named
// by its uuid.
func wantOf(spec any) (containerKind.Want, error) {
	w, err := requirements.Want[containerKind.Want](containerKind.Name, spec)
	if err != nil {
		return containerKind.Want{}, err
	}

	if len(w.VM.UUID) == 0 {
		return containerKind.Want{}, fmt.Errorf("%w: a container is required in a vm named by its uuid", kind.ErrInvalidPayload)
	}

	return w, nil
}
