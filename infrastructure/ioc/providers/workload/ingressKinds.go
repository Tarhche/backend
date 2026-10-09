package workload

import (
	"context"
	"fmt"

	ingressTasks "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/task"
	ingressVMs "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/vm"
	ingressLocateResources "github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	ingressContract "github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
)

// ingressKinds are the kinds the ingress finds the resources of, each
// registered by its ingress strategy: those with endpoints, whose ports are
// served under their slugs, and those with streams, such as a terminal. That
// is all it takes for the ingress to route them.
//
// A task and a VM have both, and are found where locations say they are,
// which is what the nodes holding them said, less what the commands on their
// way to them take away: the kinds located are theirs, each told by its own
// strategy, which knows what is said of one. A slug is asked of the kinds in
// the order they are registered, so a task's is looked for first, as it
// always was. A stack has neither: its ports are its Docker VM's, and so is
// its terminal, so the ingress finds none of it, and locates none of it.
func ingressKinds(locations ingressContract.Locations) (*kind.Registry[kind.IngressBinding], map[string]ingressLocateResources.Kind, error) {
	tasks := ingressTasks.New(locations)
	vms := ingressVMs.New(locations)

	kinds := kind.NewRegistry[kind.IngressBinding]()

	for _, binding := range []kind.IngressBinding{
		kind.BindIngress(taskKind.Descriptor(), tasks),
		kind.BindIngress(vmKind.Descriptor(), vms),
		kind.BindIngress(stackKind.Descriptor(), unreached{kind: stackKind.Name}),
	} {
		if err := kinds.Register(binding); err != nil {
			return nil, nil, err
		}
	}

	located := map[string]ingressLocateResources.Kind{
		taskKind.Name: tasks,
		vmKind.Name:   vms,
	}

	return kinds, located, nil
}

// unreached is the ingress strategy of a kind nothing is reached through the
// ingress by: it has nothing, by any uuid or slug.
type unreached struct {
	kind string
}

var _ kind.Ingress = unreached{}

func (u unreached) ByUUID(_ context.Context, uuid string) (kind.Location, error) {
	return kind.Location{}, fmt.Errorf("%w: a %s is reached through its vm, not as %q", domain.ErrNotExists, u.kind, uuid)
}

func (u unreached) BySlug(_ context.Context, slug string) (kind.Location, error) {
	return kind.Location{}, fmt.Errorf("%w: a %s is reached through its vm, not as %q", domain.ErrNotExists, u.kind, slug)
}
