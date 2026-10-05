package stopVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/workload/task/events"
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
		assert.Equal(t, vm.Stopping, stored.CurrentState)
		assert.Equal(t, []string{events.VMStopRequestedName}, w.Producer.Subjects())
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

	t.Run("a run of the code runner's is stopped as its task is", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{UUID: "run"})
		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)

		stored, ok := w.Tasks.Stored("run")
		require.True(t, ok)
		assert.Equal(t, task.Stopping, stored.CurrentState)
		assert.Equal(t, task.Stopped, stored.ExpectedState)

		var asked taskEvents.TaskStoppageRequested
		require.True(t, w.Producer.Last(taskEvents.TaskStoppageRequestedName, &asked))
		assert.Equal(t, taskEvents.TaskStoppageRequested{UUID: "run"}, asked)
	})

	t.Run("one already stopping is refused, and still wanted stopped", func(t *testing.T) {
		t.Parallel()

		run := vmtest.Run("run")
		run.CurrentState = task.Stopping

		w := vmtest.New(vmtest.WithTasks(run))

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{UUID: "run"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "invalid_state_transition"}, response.ValidationErrors)
		assert.Empty(t, w.Producer.Messages())

		stored, _ := w.Tasks.Stored("run")
		assert.Equal(t, task.Stopped, stored.ExpectedState)
	})

	t.Run("a run is nobody's own to stop", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))

		_, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "run"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, w.Producer.Messages())

		stored, _ := w.Tasks.Stored("run")
		assert.Equal(t, task.Running, stored.CurrentState)
	})

	t.Run("a vm has to be named", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"uuid": "required_field"}, response.ValidationErrors)
	})
}
