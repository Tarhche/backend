package runTask

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	tasksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/tasks"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
)

func TestTaskRan_Handle(t *testing.T) {
	t.Parallel()

	const (
		taskUUID    = "task-uuid"
		nodeName    = "workload-orchestrator-01"
		executionID = "execution-id"
	)

	started := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	// ran is what the control plane hears of a task its node says is
	// running, a run that started at startedAt with a minute to run.
	ran := func(t *testing.T, startedAt time.Time) []byte {
		t.Helper()

		event := events.TaskRan{UUID: taskUUID, NodeName: nodeName, ExecutionID: executionID, StartedAt: startedAt}
		if !startedAt.IsZero() {
			event.Deadline = startedAt.Add(time.Minute)
		}

		payload, err := json.Marshal(event)
		require.NoError(t, err)

		return payload
	}

	// running is the task as its heartbeat has already written it down by
	// the time this is heard: running, and nothing more about its run.
	running := task.Task{UUID: taskUUID, Kind: task.KindJob, TTL: time.Minute, CurrentState: task.Running, ExpectedState: task.Running}

	t.Run("a task its heartbeat already says is running is given the moment its run started", func(t *testing.T) {
		t.Parallel()

		tasks := tasksMemory.NewRepository(running)

		require.NoError(t, NewTaskRan(tasks).Handle(context.Background(), ran(t, started)))

		stored, ok := tasks.Stored(taskUUID)
		require.True(t, ok)
		assert.Equal(t, task.Running, stored.CurrentState)
		assert.Equal(t, nodeName, stored.NodeName)
		assert.Equal(t, executionID, stored.ExecutionID)
		assert.True(t, started.Equal(stored.StartedAt), "want %s, got %s", started, stored.StartedAt)
		assert.True(t, started.Add(time.Minute).Equal(stored.Deadline), "want %s, got %s", started.Add(time.Minute), stored.Deadline)
	})

	t.Run("a task coming up is running, since the moment its run started", func(t *testing.T) {
		t.Parallel()

		scheduled := running
		scheduled.CurrentState = task.Scheduled

		tasks := tasksMemory.NewRepository(scheduled)

		require.NoError(t, NewTaskRan(tasks).Handle(context.Background(), ran(t, started)))

		stored, _ := tasks.Stored(taskUUID)
		assert.Equal(t, task.Running, stored.CurrentState)
		assert.True(t, started.Equal(stored.StartedAt), "want %s, got %s", started, stored.StartedAt)
	})

	t.Run("a run that started again is counted from then", func(t *testing.T) {
		t.Parallel()

		before := running
		before.StartedAt = started
		before.Deadline = started.Add(time.Minute)

		tasks := tasksMemory.NewRepository(before)
		again := started.Add(time.Hour)

		require.NoError(t, NewTaskRan(tasks).Handle(context.Background(), ran(t, again)))

		stored, _ := tasks.Stored(taskUUID)
		assert.True(t, again.Equal(stored.StartedAt), "want %s, got %s", again, stored.StartedAt)
		assert.True(t, again.Add(time.Minute).Equal(stored.Deadline), "want %s, got %s", again.Add(time.Minute), stored.Deadline)
	})

	t.Run("a node that does not say when its run started leaves what was known", func(t *testing.T) {
		t.Parallel()

		before := running
		before.StartedAt = started

		tasks := tasksMemory.NewRepository(before)

		require.NoError(t, NewTaskRan(tasks).Handle(context.Background(), ran(t, time.Time{})))

		stored, _ := tasks.Stored(taskUUID)
		assert.True(t, started.Equal(stored.StartedAt), "want %s, got %s", started, stored.StartedAt)
	})

	t.Run("nothing is written when nothing changed", func(t *testing.T) {
		t.Parallel()

		var tasks tasksMock.MockTasksRepository

		same := running
		same.NodeName = nodeName
		same.ExecutionID = executionID
		same.StartedAt = started
		same.Deadline = started.Add(time.Minute)

		tasks.On("GetOne", mock.Anything, taskUUID).Return(same, nil)

		require.NoError(t, NewTaskRan(&tasks).Handle(context.Background(), ran(t, started)))

		tasks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})
}
