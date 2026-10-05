package restartVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("the owner's vm is asked", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "01"})
		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, vm.Restarting, stored.CurrentState)
		assert.Equal(t, []string{events.VMRestartRequestedName}, w.Producer.Subjects())
	})

	t.Run("somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		_, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "other", UUID: "01"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, w.Producer.Messages())
	})

	t.Run("one on its way out is refused", func(t *testing.T) {
		t.Parallel()

		deleting := vmtest.Running("01", "owner")
		deleting.CurrentState = vm.Deleting

		w := vmtest.New(vmtest.WithVMs(deleting))

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{UUID: "01"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "invalid_state_transition"}, response.ValidationErrors)
	})

	t.Run("a vm has to be named", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"uuid": "required_field"}, response.ValidationErrors)
	})
}

// TestUseCase_Execute_run holds a run of the code runner's to being refused
// what only a VM somebody asked for can be asked, by whoever may see it, and to
// being nobody's own.
func TestUseCase_Execute_run(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("anybody's is refused", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{UUID: "run"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": coderunner.CodeRefused}, response.ValidationErrors)
		assert.Empty(t, w.Producer.Messages())

		stored, _ := w.Tasks.Stored("run")
		assert.Equal(t, vmtest.Run("run").CurrentState, stored.CurrentState)
	})

	t.Run("one's own is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))

		_, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "run"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, w.Producer.Messages())
	})
}
