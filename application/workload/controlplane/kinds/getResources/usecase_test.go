package getResources_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resources := resourcesMemory.NewRepository()

	// 25 of alice's in house-1, and 5 of bob's in house-2.
	for i := range 30 {
		owner, house := "alice", "house-1"
		if i >= 25 {
			owner, house = "bob", "house-2"
		}

		_, err := resources.Create(ctx, kindstest.AFan(fmt.Sprintf("fan-%02d", i), kindstest.Running, kindstest.Running, func(f *kindstest.Fan) {
			f.Metadata.OwnerUUID = owner
			f.Metadata.Owners = []kind.Reference{{Kind: kindstest.Parent, UUID: house}}
		}))
		require.NoError(t, err)
	}

	useCase := getResources.NewUseCase(kindstest.Registry(&kindstest.Fans{}), resources)

	for name, tt := range map[string]struct {
		request    getResources.Request
		first      string
		count      int
		pagination presenter.Pagination
	}{
		"anybody's, newest first, a page at a time": {
			request:    getResources.Request{Kind: kindstest.Kind},
			first:      "fan-29",
			count:      20,
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 1},
		},
		"and the page after": {
			request:    getResources.Request{Kind: kindstest.Kind, Page: 2},
			first:      "fan-09",
			count:      10,
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 2},
		},
		"one person's own": {
			request:    getResources.Request{Kind: kindstest.Kind, OwnerUUID: "bob"},
			first:      "fan-29",
			count:      5,
			pagination: presenter.Pagination{TotalPages: 1, CurrentPage: 1},
		},
		"those living in one resource": {
			request:    getResources.Request{Kind: kindstest.Kind, Parent: "house-1", Page: 2},
			first:      "fan-04",
			count:      5,
			pagination: presenter.Pagination{TotalPages: 2, CurrentPage: 2},
		},
		"nobody's is none": {
			request:    getResources.Request{Kind: kindstest.Kind, OwnerUUID: "carol"},
			pagination: presenter.Pagination{TotalPages: 0, CurrentPage: 1},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(ctx, &tt.request)
			require.NoError(t, err)

			require.Len(t, response.Items, tt.count)
			if tt.count > 0 {
				assert.Equal(t, tt.first, response.Items[0].Metadata.UUID)
			}

			assert.Equal(t, tt.pagination, response.Pagination)
		})
	}

	t.Run("a kind not run here has nothing to list", func(t *testing.T) {
		t.Parallel()

		_, err := useCase.Execute(ctx, &getResources.Request{Kind: "kettle"})

		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})
}
