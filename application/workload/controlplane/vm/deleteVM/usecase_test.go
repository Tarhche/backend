package deleteVM

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
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

	t.Run("its node is asked to remove it, and the record waits", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "01"})
		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)

		stored, ok := w.VMs.Stored("01")
		require.True(t, ok)
		assert.Equal(t, vm.Deleting, stored.CurrentState)

		var asked events.VMDeleteRequested
		require.True(t, w.Producer.Last(events.VMDeleteRequestedName, &asked))
		assert.Equal(t, events.VMDeleteRequested{VMUUID: "01", NodeName: vmtest.Node}, asked)
	})

	t.Run("somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		_, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "other", UUID: "01"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a run of the code runner's is taken away as its task is, running or not", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))
		require.NoError(t, w.TaskLogs.Append(ctx, []task.Log{{TaskUUID: "run", LogLine: task.LogLine{Content: "hello"}}}))

		response, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{UUID: "run"})
		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)

		_, kept := w.Tasks.Stored("run")
		assert.False(t, kept)

		logs, err := w.TaskLogs.Get(ctx, "run", time.Time{}, 10)
		require.NoError(t, err)
		assert.Empty(t, logs)

		var asked taskEvents.TaskDeleted
		require.True(t, w.Producer.Last(taskEvents.TaskDeletedName, &asked))
		assert.Equal(t, "run", asked.UUID)
	})

	t.Run("a run is nobody's own to delete", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))

		_, err := NewUseCase(w.VMs, w.Runs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "run"})
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, kept := w.Tasks.Stored("run")
		assert.True(t, kept)
	})
}

func deletedEvent(t *testing.T, uuid string) []byte {
	t.Helper()

	payload, err := json.Marshal(events.VMDeleted{VMUUID: uuid, NodeName: vmtest.Node})
	require.NoError(t, err)

	return payload
}

func TestVMDeleted_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a vm its node no longer holds is forgotten, with its stacks", func(t *testing.T) {
		t.Parallel()

		deleting := vmtest.Running("01", "owner")
		deleting.CurrentState = vm.Deleting

		w := vmtest.New(vmtest.WithVMs(deleting), vmtest.WithStacks(stack.Stack{UUID: "s1", VMUUID: "01", Slug: "web-a"}))

		require.NoError(t, NewVMDeleted(w.VMs, w.Lifecycle, slog.New(slog.DiscardHandler)).Handle(ctx, deletedEvent(t, "01")))

		_, kept := w.VMs.Stored("01")
		assert.False(t, kept)

		_, kept = w.Stacks.Stored("s1")
		assert.False(t, kept)
	})

	t.Run("one that was not asked to go is not forgotten", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		require.NoError(t, NewVMDeleted(w.VMs, w.Lifecycle, slog.New(slog.DiscardHandler)).Handle(ctx, deletedEvent(t, "01")))

		_, kept := w.VMs.Stored("01")
		assert.True(t, kept)
	})

	t.Run("what will never be handled is not handed back", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()
		handler := NewVMDeleted(w.VMs, w.Lifecycle, slog.New(slog.DiscardHandler))

		assert.NoError(t, handler.Handle(ctx, []byte("{not json")))
		assert.NoError(t, handler.Handle(ctx, deletedEvent(t, "missing")))
	})
}
