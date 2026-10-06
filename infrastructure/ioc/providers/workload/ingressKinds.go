package workload

import (
	"context"
	"fmt"

	ingressTasks "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/task"
	ingressVMs "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain"
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
// A task and a VM have both, and are found by their records, which records
// keeps. A slug is asked of the kinds in the order they are registered, so a
// task's is looked for first, as it always was. A stack has neither: its
// ports are its Docker VM's, and so is its terminal, so the ingress finds
// none of it.
func ingressKinds(records ingressVMs.Records) (*kind.Registry[kind.IngressBinding], error) {
	kinds := kind.NewRegistry[kind.IngressBinding]()

	for _, binding := range []kind.IngressBinding{
		kind.BindIngress(taskKind.Descriptor(), ingressTasks.New(records)),
		kind.BindIngress(vmKind.Descriptor(), ingressVMs.New(records)),
		kind.BindIngress(stackKind.Descriptor(), unreached{kind: stackKind.Name}),
	} {
		if err := kinds.Register(binding); err != nil {
			return nil, err
		}
	}

	return kinds, nil
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
