package stackindex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func page(current uint, total uint, slugs ...string) workloadControlPlane.Page[stack.Stack] {
	items := make([]stack.Stack, len(slugs))
	for i, slug := range slugs {
		items[i] = stack.Stack{UUID: slug + "-uuid", Slug: slug}
	}

	return workloadControlPlane.Page[stack.Stack]{Items: items, CurrentPage: current, TotalPages: total}
}

func TestOf(t *testing.T) {
	t.Parallel()

	t.Run("every page of the stacks the containers were listed beside", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Stacks", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", uint(1)).Once().Return(page(1, 2, "shop"), nil)
		workload.On("Stacks", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", uint(2)).Once().Return(page(2, 2, "blog"), nil)
		defer workload.AssertExpectations(t)

		index := Of(context.Background(), &workload, workloadtest.OwnerUUID, "vm-uuid")

		assert.Equal(t, Index{"shop": "shop-uuid", "blog": "blog-uuid"}, index)
	})

	t.Run("no more than so many pages for one request", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Stacks", mock.Anything, "", "", mock.Anything).Times(maxPages).Return(page(1, 1000, "shop"), nil)
		defer workload.AssertExpectations(t)

		assert.Equal(t, Index{"shop": "shop-uuid"}, Of(context.Background(), &workload, "", ""))
	})

	t.Run("a listing that cannot be read is an index of nothing, not a failure", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Stacks", mock.Anything, "", "vm-uuid", uint(1)).Once().Return(workloadControlPlane.Page[stack.Stack]{}, workloadtest.ErrUnreachable)
		defer workload.AssertExpectations(t)

		assert.Empty(t, Of(context.Background(), &workload, "", "vm-uuid"))
	})
}

func TestIndex_Link(t *testing.T) {
	t.Parallel()

	containers := []presenter.Container{
		{ID: "a", Stack: "shop"},
		{ID: "b"},
		{ID: "c", Stack: "by-hand"},
	}

	Index{"shop": "shop-uuid"}.Link(containers)

	assert.Equal(t, "shop-uuid", containers[0].StackUUID)
	assert.Empty(t, containers[1].StackUUID, "no stack deployed it")
	assert.Empty(t, containers[2].StackUUID, "somebody ran compose for it by hand")
}
