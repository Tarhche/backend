package getVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	box := vmtest.Running("vm-uuid", "owner")

	// a task of somebody else's is no run of the code runner's.
	notARun := vmtest.Run("task-uuid")
	notARun.OwnerUUID = "owner"

	w := vmtest.New(vmtest.WithVMs(box), vmtest.WithTasks(vmtest.Run("run-uuid"), notARun))
	useCase := NewUseCase(w.VMs, w.Runs)

	for name, tt := range map[string]struct {
		request   Request
		wantErr   error
		name      string
		managedBy string
	}{
		"anybody's":                 {request: Request{UUID: "vm-uuid"}, name: "box"},
		"its owner's own":           {request: Request{OwnerUUID: "owner", UUID: "vm-uuid"}, name: "box"},
		"not somebody else's":       {request: Request{OwnerUUID: "other", UUID: "vm-uuid"}, wantErr: domain.ErrNotExists},
		"not one that is not there": {request: Request{UUID: "missing"}, wantErr: domain.ErrNotExists},
		"a run, to anybody": {
			request:   Request{UUID: "run-uuid"},
			name:      "request-run-uuid",
			managedBy: vm.ManagedByCodeRunner,
		},
		"no run is anybody's own": {
			request: Request{OwnerUUID: task.GuestOwnerUUID, UUID: "run-uuid"},
			wantErr: domain.ErrNotExists,
		},
		"nor somebody else's own": {
			request: Request{OwnerUUID: "owner", UUID: "run-uuid"},
			wantErr: domain.ErrNotExists,
		},
		"a task that is not the code runner's is not one": {
			request: Request{UUID: "task-uuid"},
			wantErr: domain.ErrNotExists,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(context.Background(), &tt.request)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.name, response.Name)
			assert.Equal(t, "running", response.State)
			assert.Equal(t, tt.managedBy, response.ManagedBy)
		})
	}
}
