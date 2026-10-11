package stackindex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
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

		assert.Equal(t, Index{"shop-uuid": "shop", "blog-uuid": "blog"}, index)
	})

	t.Run("no more than so many pages for one request", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Stacks", mock.Anything, "", "", mock.Anything).Times(maxPages).Return(page(1, 1000, "shop"), nil)
		defer workload.AssertExpectations(t)

		assert.Equal(t, Index{"shop-uuid": "shop"}, Of(context.Background(), &workload, "", ""))
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

	containers := []docker.Container{
		{ID: "a", Stack: "shop-uuid", Service: "web"},
		{ID: "b"},
		{ID: "c", Stack: "unseen-uuid", Service: "web"},
	}

	presented := presenter.NewContainers(containers)

	Index{"shop-uuid": "shop"}.Link(presented, containers)

	assert.Equal(t, "shop", presented[0].Stack, "named by its slug")
	assert.Equal(t, "shop-uuid", presented[0].StackUUID, "and its uuid")

	assert.Empty(t, presented[1].Stack, "no stack made it")
	assert.Empty(t, presented[1].StackUUID)

	assert.Empty(t, presented[2].Stack, "whoever asked cannot see its stack, which is neither named nor linked")
	assert.Empty(t, presented[2].StackUUID)
	assert.Equal(t, "web", presented[2].Service, "though it says which service it is, as its labels do")
}

func TestIndex_Name(t *testing.T) {
	t.Parallel()

	index := Index{"shop-uuid": "shop"}

	var one presenter.Container
	index.Name(&one, "shop-uuid")
	assert.Equal(t, presenter.Container{Stack: "shop", StackUUID: "shop-uuid"}, one)

	var none presenter.Container
	index.Name(&none, "")
	index.Name(&none, "unseen-uuid")
	assert.Equal(t, presenter.Container{}, none, "what no stack it can see made is left as it was")
}
