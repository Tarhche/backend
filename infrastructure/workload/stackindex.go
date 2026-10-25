package workload

import (
	"context"
	"maps"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
)

// maxPages bounds how much of a listing of stacks is read for one request.
// A container whose stack is past it is shown without its stack, which a
// client does without.
const maxPages = 10

// StackIndex is the stacks a request may see: each one's slug, by its uuid.
type StackIndex map[string]string

// StackIndexOf indexes the stacks ownerUUID has (anybody's when it is empty),
// in one VM or in every one when vmUUID is empty: the same narrowing the
// containers beside them were listed with, so a container is only ever linked
// to a stack whoever asked can see.
//
// A listing that cannot be read is an index of nothing. Which stack made a
// container is worth saying, not worth failing over.
func StackIndexOf(ctx context.Context, workload workloadControlPlane.Client, ownerUUID string, vmUUID string) StackIndex {
	index := make(StackIndex)

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

// NameStacks names the stack of every presented container that has one in
// the index: presented are containers, as the dashboard shows them, in the
// same order.
func (i StackIndex) NameStacks(presented []presenter.Container, containers []docker.Container) {
	for c := range presented {
		i.NameStack(&presented[c], containers[c].Stack)
	}
}

// NameStack names the stack whose uuid a container says made it, its slug and
// its uuid, when the index has it. A container no stack made names none; and
// one of a stack whoever asked cannot see names none either, not even by the
// label that carries its stack's uuid, which is left out of its labels, as its
// stack is: the service of the stack's file it is, which its labels say too,
// is said all the same.
func (i StackIndex) NameStack(container *presenter.Container, stackUUID string) {
	if slug, seen := i[stackUUID]; seen && len(stackUUID) > 0 {
		container.Stack, container.StackUUID = slug, stackUUID

		return
	}

	if _, labelled := container.Labels[stackKind.LabelStack]; labelled {
		labels := maps.Clone(container.Labels)
		delete(labels, stackKind.LabelStack)

		container.Labels = labels
	}
}
