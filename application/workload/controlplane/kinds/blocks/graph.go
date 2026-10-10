package blocks

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Recorded is the record of the kind kept in the Docker VM vmUUID names that
// was asked for under name, its Docker name, which is what a graph knows it
// by, and whether there is one: one that is not on its way out before one
// that is, which one of the same name may be about to take the place of.
func (b *Blocks) Recorded(ctx context.Context, vmUUID string, name string) (resource.Record, bool, error) {
	kept, _, err := b.Resources.GetAll(ctx, b.descriptor.Name, resource.Filter{Parent: kind.Reference{Kind: blockKinds.Parent, UUID: vmUUID}}, 0, 0)
	if err != nil {
		return resource.Record{}, false, err
	}

	going := -1

	for i := range kept {
		if !b.kind.Named(kept[i].Raw, name) {
			continue
		}

		common, err := kept[i].Common()
		if err != nil {
			return resource.Record{}, false, err
		}

		if !requirements.Going(common) {
			return kept[i], true, nil
		}

		if going < 0 {
			going = i
		}
	}

	if going < 0 {
		return resource.Record{}, false, nil
	}

	return kept[going], true, nil
}

// Ready is nothing when what lives in the Docker VM vmUUID names can be made
// or removed now, which it can while the VM runs. One that does not, and one
// being deleted, which takes what lives in it with it, are graph.Waiting,
// saying why; one that is not there is domain.ErrNotExists.
func (b *Blocks) Ready(ctx context.Context, vmUUID string) error {
	v, err := b.VMs.GetOne(ctx, vmUUID)
	switch {
	case err != nil:
		return err
	case records.Going(v):
		return &graph.Waiting{Reason: "its vm is being deleted"}
	case v.Status.State != vmKind.Running:
		return &graph.Waiting{Reason: fmt.Sprintf("its vm is %s", v.Status.State)}
	}

	return nil
}

// ParentOf is the uuid of the Docker VM a building block a graph needs goes
// into: the record the graph made of the VM its choice names, among those of
// what it needs, by their nodes, or else the one its choice names.
func ParentOf(choice stackKind.VMChoice, deps map[graph.NodeID]graph.Ref) string {
	if made, there := deps[graph.ID(vmKind.Name, choice.UUID)]; there && made.Kind == vmKind.Name && len(made.UUID) > 0 {
		return made.UUID
	}

	return choice.UUID
}
