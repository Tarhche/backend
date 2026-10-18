package stackindex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
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

	// labelled as the platform labels a stack's container, its stack's uuid
	// and its service.
	labelled := func(stackUUID string) map[string]string {
		return map[string]string{"workload.managed": "true", "workload.stack": stackUUID, "workload.stack.service": "web"}
	}

	containers := []docker.Container{
		{ID: "a", Stack: "shop-uuid", Service: "web", Labels: labelled("shop-uuid")},
		{ID: "b", Labels: map[string]string{"made": "by hand"}},
		{ID: "c", Stack: "unseen-uuid", Service: "web", Labels: labelled("unseen-uuid")},
	}

	presented := presenter.NewContainers(containers)

	Index{"shop-uuid": "shop"}.Link(presented, containers)

	assert.Equal(t, "shop", presented[0].Stack, "named by its slug")
	assert.Equal(t, "shop-uuid", presented[0].StackUUID, "and its uuid")
	assert.Equal(t, labelled("shop-uuid"), presented[0].Labels, "as its labels say")

	assert.Empty(t, presented[1].Stack, "no stack made it")
	assert.Empty(t, presented[1].StackUUID)
	assert.Equal(t, map[string]string{"made": "by hand"}, presented[1].Labels)

	assert.Empty(t, presented[2].Stack, "whoever asked cannot see its stack, which is neither named nor linked")
	assert.Empty(t, presented[2].StackUUID)
	assert.Equal(t, map[string]string{"workload.managed": "true", "workload.stack.service": "web"}, presented[2].Labels, "nor said by its labels")
	assert.Equal(t, "web", presented[2].Service, "though it says which service it is, as its labels do")

	assert.Equal(t, labelled("unseen-uuid"), containers[2].Labels, "and what it was read as is left as it was")
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
