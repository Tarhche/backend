package getVMs

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	// 25 of the owner's, every fifth a Docker VM, and one of somebody else's.
	var stored []vm.VM
	for i := range 25 {
		kind := vm.KindMachine
		if i%5 == 0 {
			kind = vm.KindDocker
		}

		stored = append(stored, vm.VM{UUID: fmt.Sprintf("%02d", i), OwnerUUID: "owner", Kind: kind})
	}

	stored = append(stored, vm.VM{UUID: "99", OwnerUUID: "other", Kind: vm.KindDocker})

	useCase := NewUseCase(vmsMemory.NewRepository(stored...))

	for name, tt := range map[string]struct {
		request    Request
		count      int
		first      string
		totalPages uint
		page       uint
	}{
		"anybody's, the first page": {
			request:    Request{},
			count:      20,
			first:      "99",
			totalPages: 2,
			page:       1,
		},
		"page zero is the first page": {
			request:    Request{Page: 0},
			count:      20,
			totalPages: 2,
			first:      "99",
			page:       1,
		},
		"one person's, the second page": {
			request:    Request{OwnerUUID: "owner", Page: 2},
			count:      5,
			first:      "04",
			totalPages: 2,
			page:       2,
		},
		"one person's docker vms": {
			request:    Request{OwnerUUID: "owner", Kind: vm.KindDocker},
			count:      5,
			first:      "20",
			totalPages: 1,
			page:       1,
		},
		"anybody's docker vms": {
			request:    Request{Kind: vm.KindDocker},
			count:      6,
			first:      "99",
			totalPages: 1,
			page:       1,
		},
		"past the last page": {
			request:    Request{Kind: vm.KindDocker, Page: 3},
			count:      0,
			totalPages: 1,
			page:       3,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(context.Background(), &tt.request)
			require.NoError(t, err)

			assert.Len(t, response.Items, tt.count)
			assert.Equal(t, tt.totalPages, response.Pagination.TotalPages)
			assert.Equal(t, tt.page, response.Pagination.CurrentPage)

			if tt.count > 0 {
				assert.Equal(t, tt.first, response.Items[0].UUID)
			}
		})
	}
}
