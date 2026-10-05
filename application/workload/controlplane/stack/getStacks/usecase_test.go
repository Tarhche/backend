package getStacks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	web := stack.Stack{UUID: "s1", Name: "web", OwnerUUID: "owner", VMUUID: "d1", Slug: "web-abcde", State: stack.Running}
	api := stack.Stack{UUID: "s2", Name: "api", OwnerUUID: "owner", VMUUID: "d1", Slug: "api-abcde", State: stack.Running}
	shop := stack.Stack{UUID: "s3", Name: "shop", OwnerUUID: "other", VMUUID: "d2", Slug: "shop-abcde", State: stack.Running}
	orphan := stack.Stack{UUID: "s4", Name: "orphan", OwnerUUID: "owner", VMUUID: "gone", Slug: "orphan-abcde", State: stack.Failed}

	builds := vmtest.Docker("d1", "owner")
	builds.Name = "builds"

	theirs := vmtest.Docker("d2", "other")
	theirs.Name = "theirs"

	names := func(response *Response) map[string]string {
		named := make(map[string]string, len(response.Items))
		for _, item := range response.Items {
			named[item.UUID] = item.VMName
		}

		return named
	}

	t.Run("each stack says what its vm is called", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(builds, theirs), vmtest.WithStacks(web, api, shop, orphan))

		everybody, err := NewUseCase(w.Stacks, w.VMs).Execute(ctx, &Request{Page: 1})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"s1": "builds", "s2": "builds", "s3": "theirs", "s4": ""}, names(everybody), "one whose vm is gone is named by nothing")

		mine, err := NewUseCase(w.Stacks, w.VMs).Execute(ctx, &Request{OwnerUUID: "owner", Page: 1})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"s1": "builds", "s2": "builds", "s4": ""}, names(mine))

		inOne, err := NewUseCase(w.Stacks, w.VMs).Execute(ctx, &Request{VMUUID: "d1", Page: 1})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"s1": "builds", "s2": "builds"}, names(inOne))
	})

	t.Run("a vm renamed is named anew, since the name is read rather than kept", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(builds), vmtest.WithStacks(web))

		renamed, err := w.VMs.GetOne(ctx, "d1")
		require.NoError(t, err)

		renamed.Name = "ci"
		_, err = w.VMs.Save(ctx, &renamed)
		require.NoError(t, err)

		response, err := NewUseCase(w.Stacks, w.VMs).Execute(ctx, &Request{Page: 1})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"s1": "ci"}, names(response))

		kept, err := w.Stacks.GetOne(ctx, "s1")
		require.NoError(t, err)
		assert.Empty(t, kept.VMName, "the name is never kept with the stack")
	})
}
