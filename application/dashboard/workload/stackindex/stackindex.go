// Package stackindex says which stack deployed a container.
//
// A container knows only the compose project docker labelled it with, which
// is a stack's slug; the dashboard links a container to its stack by the
// stack's uuid. This looks the stacks up once per request and matches the two
// up, so a listing of containers costs one more question, not one per row.
package stackindex

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// maxPages bounds how much of a listing of stacks is read for one request.
// A container whose stack is past it is shown without its stack's uuid,
// which a client does without.
const maxPages = 10

// Index is the stacks a request may see, by slug.
type Index map[string]string

// Of indexes the stacks ownerUUID has (anybody's when it is empty), in one VM
// or in every one when vmUUID is empty: the same narrowing the containers
// beside them were listed with, so a container is only ever linked to a stack
// whoever asked can see.
//
// A listing that cannot be read is an index of nothing. Which stack deployed
// a container is worth saying, not worth failing over.
func Of(ctx context.Context, workload workloadControlPlane.Client, ownerUUID string, vmUUID string) Index {
	index := make(Index)

	for page := uint(1); page <= maxPages; page++ {
		stacks, err := workload.Stacks(ctx, ownerUUID, vmUUID, page)
		if err != nil {
			return index
		}

		for _, s := range stacks.Items {
			index[s.Slug] = s.UUID
		}

		if page >= stacks.TotalPages {
			return index
		}
	}

	return index
}

// Link names the stack of every container that has one in the index.
func (i Index) Link(containers []presenter.Container) {
	for c := range containers {
		containers[c].StackUUID = i.Of(containers[c].Stack)
	}
}

// Of is the uuid of the stack with that slug, and nothing for a container no
// stack deployed or one somebody ran compose for by hand.
func (i Index) Of(slug string) string {
	if len(slug) == 0 {
		return ""
	}

	return i[slug]
}
