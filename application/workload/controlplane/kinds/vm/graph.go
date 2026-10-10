package vm

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// tries is how many times a VM a graph needs is looked for and made, when
// another made it under its uuid in the meantime.
const tries = 2

// Graph is the vm kind as a graph needs it (graph.Kind): the Docker VM what
// lives in it goes into, named by the uuid what needs it holds for it, and
// made under that uuid, through the chooser, whenever it is not there, with
// the Docker defaults and whatever else it is wanted with (vmKind.Want). It
// needs nothing.
//
// A VM is there while its record is and is not on its way out, whatever it is
// doing: keeping it what it was asked to be, starting it again or making it
// again on its node, is its record's own loop's, and what lives in it waits
// while it does not run. One a graph made is made once its create is
// answered, and it runs; one its node could not make is not made, saying why,
// and is there from then on, its own loop asking for it again.
type Graph struct {
	chooser *dockervm.Chooser
	records *requirements.Records
}

var _ graph.Kind = &Graph{}

// NewGraph is the vm kind as a graph needs it, making Docker VMs through
// chooser, and keeping and removing them through records.
func NewGraph(chooser *dockervm.Chooser, records *requirements.Records) *Graph {
	return &Graph{chooser: chooser, records: records}
}

// Requires is nothing: a VM needs nothing a graph makes.
func (g *Graph) Requires(spec any) ([]graph.Requirement, error) {
	if _, err := wantOf(spec); err != nil {
		return nil, err
	}

	return nil, nil
}

// Exists is the VM the want names, when its record is there and is not on
// its way out.
func (g *Graph) Exists(ctx context.Context, spec any) (graph.Ref, bool, error) {
	w, err := wantOf(spec)
	if err != nil {
		return graph.Ref{}, false, err
	}

	v, there, err := g.read(ctx, w.VM.UUID)
	if err != nil || !there || records.Going(v) {
		return graph.Ref{}, false, err
	}

	return refOf(v), true, nil
}

// Create makes the Docker VM the want names under its uuid when it is not
// there, and is it once its node has made it. One there already is it,
// whatever it is doing, once what came of its create is heard when it is
// being made; one on its way out is waited on until it is gone.
func (g *Graph) Create(ctx context.Context, spec any, _ map[graph.NodeID]graph.Ref) (graph.Ref, error) {
	w, err := wantOf(spec)
	if err != nil {
		return graph.Ref{}, err
	}

	for try := 1; ; try++ {
		r, there, err := g.records.Read(ctx, vmKind.Name, w.VM.UUID)
		if err != nil {
			return graph.Ref{}, err
		}

		if there {
			return g.there(ctx, r)
		}

		_, refused, err := g.chooser.Make(ctx, w.OwnerUUID, w.VM.UUID, choiceOf(w.VM))

		switch {
		case errors.Is(err, domain.ErrAlreadyExists) && try < tries:
			continue
		case err != nil:
			return graph.Ref{}, err
		case len(refused) > 0:
			return graph.Ref{}, requirements.Refused(vmKind.Name, refused)
		}

		made, err := g.records.Settled(ctx, vmKind.Name, w.VM.UUID)
		if err != nil {
			return graph.Ref{}, err
		}

		v, err := records.Decode(made)
		if err != nil {
			return graph.Ref{}, err
		}

		if !madeBy(v) {
			return graph.Ref{}, requirements.NotDone(made, "made")
		}

		return refOf(v), nil
	}
}

// there is a VM whose record is there: it, once its node has said what came
// of its create when it is being made, whatever it is doing then. One on its
// way out is waited on until it is gone.
func (g *Graph) there(ctx context.Context, r resource.Record) (graph.Ref, error) {
	v, err := records.Decode(r)
	if err != nil {
		return graph.Ref{}, err
	}

	if v.Status.State == vmKind.Scheduled && r.Pending != nil && !records.Going(v) {
		settled, went, err := g.records.Settle(ctx, r)
		switch {
		case err != nil:
			return graph.Ref{}, err
		case went:
			return graph.Ref{}, requirements.Gone(r)
		}

		if v, err = records.Decode(settled); err != nil {
			return graph.Ref{}, err
		}
	}

	if records.Going(v) {
		return graph.Ref{}, &graph.Waiting{Reason: "it is being deleted"}
	}

	return refOf(v), nil
}

// Delete removes the VM ref names, and is done once its node has: one gone
// already is removed. What lives in it goes with it, as a VM's delete always
// takes it.
func (g *Graph) Delete(ctx context.Context, ref graph.Ref) error {
	if ref.Kind != vmKind.Name {
		return fmt.Errorf("%w: a %s is not a vm", kind.ErrUnknownKind, ref.Kind)
	}

	return g.records.Remove(ctx, ref, nil)
}

// read is the VM uuid names, and whether there is one.
func (g *Graph) read(ctx context.Context, uuid string) (vmKind.VM, bool, error) {
	r, there, err := g.records.Read(ctx, vmKind.Name, uuid)
	if err != nil || !there {
		return vmKind.VM{}, false, err
	}

	v, err := records.Decode(r)
	if err != nil {
		return vmKind.VM{}, false, err
	}

	return v, true, nil
}

// madeBy reports whether a VM's node has made it, as its record says once
// its create is answered: anything but created or scheduled, which it is
// before its node has said, or failed before its node ever gave it what it
// runs with (Status.Applied), which a VM its node made always has. One on its
// way out is not.
func madeBy(v vmKind.VM) bool {
	switch {
	case records.Going(v):
		return false
	case v.Status.State == vmKind.Created, v.Status.State == vmKind.Scheduled:
		return false
	case v.Status.State == vmKind.Failed:
		return v.Status.Applied != nil
	}

	return true
}

// refOf is the record a VM a graph needs is.
func refOf(v vmKind.VM) graph.Ref {
	return graph.Ref{Kind: vmKind.Name, UUID: v.Metadata.UUID}
}

// wantOf is spec as a VM a graph needs, which is named by its uuid.
func wantOf(spec any) (vmKind.Want, error) {
	w, err := requirements.Want[vmKind.Want](vmKind.Name, spec)
	if err != nil {
		return vmKind.Want{}, err
	}

	if len(w.VM.UUID) == 0 {
		return vmKind.Want{}, fmt.Errorf("%w: a vm is required by its uuid", kind.ErrInvalidPayload)
	}

	return w, nil
}

// choiceOf is what a VM a graph needs is made with, as the chooser reads it.
func choiceOf(choice stackKind.VMChoice) dockervm.Choice {
	chosen := dockervm.Choice{Name: choice.Name, Ports: choice.Ports}

	if choice.Resources != nil {
		chosen.Resources = &vmKind.Resources{CPUs: choice.Resources.CPUs, Memory: choice.Resources.Memory, Disk: choice.Resources.Disk}
	}

	if choice.Network != nil {
		chosen.Network = &vmKind.Network{Ingress: vm.Access(choice.Network.Ingress), Egress: vm.Access(choice.Network.Egress)}
	}

	return chosen
}
