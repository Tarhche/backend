// Package stackindex says which stack made a container.
//
// A container knows the uuid of the stack whose graph made it, which its
// labels carry (docker.Container.Stack); the dashboard shows a container with
// its stack's slug and its uuid, and only when whoever asked can see that
// stack. This looks the stacks up once per request and matches the two up, so
// a listing of containers costs one more question, not one per row.
package stackindex

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// maxPages bounds how much of a listing of stacks is read for one request.
// A container whose stack is past it is shown without its stack, which a
// client does without.
const maxPages = 10

// Index is the stacks a request may see: each one's slug, by its uuid.
type Index map[string]string

// Of indexes the stacks ownerUUID has (anybody's when it is empty), in one VM
// or in every one when vmUUID is empty: the same narrowing the containers
// beside them were listed with, so a container is only ever linked to a stack
// whoever asked can see.
//
// A listing that cannot be read is an index of nothing. Which stack made a
// container is worth saying, not worth failing over.
func Of(ctx context.Context, workload workloadControlPlane.Client, ownerUUID string, vmUUID string) Index {
	index := make(Index)

	for page := uint(1); page <= maxPages; page++ {
		stacks, err := workload.Stacks(ctx, ownerUUID, vmUUID, page)
		if err != nil {
			return index
		}

		for _, s := range stacks.Items {
			index[s.UUID] = s.Slug
		}

		if page >= stacks.TotalPages {
			return index
		}
	}

	return index
}

// Link names the stack of every presented container that has one in the
// index: presented are containers, as the dashboard shows them, in the same
// order.
func (i Index) Link(presented []presenter.Container, containers []docker.Container) {
	for c := range presented {
		i.Name(&presented[c], containers[c].Stack)
	}
}

// Name names the stack whose uuid a container says made it, its slug and its
// uuid, when the index has it; a container no stack made, or one of a stack
// whoever asked cannot see, is left as it was.
func (i Index) Name(container *presenter.Container, stackUUID string) {
	if len(stackUUID) == 0 {
		return
	}

	slug, seen := i[stackUUID]
	if !seen {
		return
	}

	container.Stack, container.StackUUID = slug, stackUUID
}
