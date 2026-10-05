package getVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	repository := vmsMemory.NewRepository(vm.VM{UUID: "vm-uuid", Name: "box", OwnerUUID: "owner", CurrentState: vm.Running, ExpectedState: vm.Running})
	useCase := NewUseCase(repository)

	for name, tt := range map[string]struct {
		request Request
		wantErr error
	}{
		"anybody's":                 {request: Request{UUID: "vm-uuid"}},
		"its owner's own":           {request: Request{OwnerUUID: "owner", UUID: "vm-uuid"}},
		"not somebody else's":       {request: Request{OwnerUUID: "other", UUID: "vm-uuid"}, wantErr: domain.ErrNotExists},
		"not one that is not there": {request: Request{UUID: "missing"}, wantErr: domain.ErrNotExists},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(context.Background(), &tt.request)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "box", response.Name)
			assert.Equal(t, "running", response.State)
		})
	}
}
