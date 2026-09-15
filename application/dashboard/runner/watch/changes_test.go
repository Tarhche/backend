package watch

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	runnerStack "github.com/khanzadimahdi/testproject/domain/runner/stack"
	stackEvents "github.com/khanzadimahdi/testproject/domain/runner/stack/events"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/domain/user"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	runnerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

const (
	taskUUID      = "task-uuid"
	stackUUID     = "stack-uuid"
	ownerUUID     = "owner-uuid"
	somebodyElse  = "somebody-else"
	ingressDomain = "runner.example.com"

	everybodysWatch = "everybodys-watch"
	ownWatch        = "own-watch"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func directory() *presenter.Directory {
	var userRepository users.MockUsersRepository
	userRepository.On("GetByUUIDs", mock.Anything, mock.Anything).Return([]user.User{{UUID: ownerUUID}}, nil).Maybe()

	return presenter.NewDirectory(&userRepository)
}

func changes(t *testing.T, runner *runnerMock.MockClient, replyer *messagingMock.RecordingReplyer) (*Changes, *Watchers) {
	t.Helper()

	watchers := NewWatchers()

	return NewChanges(watchers, runner, directory(), replyer, ingressDomain, discardLogger()), watchers
}

func beat(t *testing.T, heartbeat taskEvents.Heartbeat) []byte {
	t.Helper()

	payload, err := json.Marshal(heartbeat)
	require.NoError(t, err)

	return payload
}

func running() taskEvents.Heartbeat {
	return taskEvents.Heartbeat{
		UUID:      taskUUID,
		Name:      "nginx",
		Slug:      "nginx-xkfqz",
		Kind:      string(task.KindService),
		OwnerUUID: ownerUUID,
		Image:     "nginx:alpine",
		State:     int(task.Running),
		NodeName:  "worker-a",
		At:        time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC),
	}
}

// taskChanges is what each reply says about a task, in order.
func taskChanges(t *testing.T, replyer *messagingMock.RecordingReplyer) []TaskChange {
	t.Helper()

	replies := replyer.Replies()
	changes := make([]TaskChange, len(replies))

	for i, reply := range replies {
		require.NoError(t, json.Unmarshal(reply.Payload, &changes[i]))
	}

	return changes
}

func TestChanges_Heartbeat(t *testing.T) {
	t.Parallel()

	t.Run("a watch is told what a task is now", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchTasks(everybodysWatch, "")

		require.NoError(t, c.Heartbeat(context.Background(), beat(t, running())))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, everybodysWatch, replies[0].RequestID)
		assert.Equal(t, domain.ReplyChunk, replies[0].Kind, "a watch is a stream, so a change is a piece of one")

		change := taskChanges(t, &replyer)[0]
		assert.Equal(t, kindChanged, change.Kind)
		assert.Equal(t, taskUUID, change.UUID)
		require.NotNil(t, change.Task)
		assert.Equal(t, task.Running.String(), change.Task.State)
		assert.Equal(t, "worker-a", change.Task.NodeName)
	})

	t.Run("a beat that repeats the last one says nothing", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchTasks(everybodysWatch, "")

		first := running()
		require.NoError(t, c.Heartbeat(context.Background(), beat(t, first)))

		// the same task, a second later: a node reports what it holds over
		// and over, and almost none of it is news.
		again := first
		again.At = first.At.Add(time.Second)
		require.NoError(t, c.Heartbeat(context.Background(), beat(t, again)))

		assert.Len(t, replyer.Replies(), 1)

		// and what it became is news again.
		stopped := again
		stopped.State = int(task.Stopped)
		stopped.At = again.At.Add(time.Second)
		require.NoError(t, c.Heartbeat(context.Background(), beat(t, stopped)))

		require.Len(t, replyer.Replies(), 2)
		assert.Equal(t, task.Stopped.String(), taskChanges(t, &replyer)[1].Task.State)
	})

	t.Run("a watch on one's own tasks hears only about those", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchTasks(ownWatch, somebodyElse)

		require.NoError(t, c.Heartbeat(context.Background(), beat(t, running())))

		assert.Empty(t, replyer.Replies(), "a task that is not theirs is not their news")

		theirs := running()
		theirs.UUID = "their-task"
		theirs.OwnerUUID = somebodyElse
		require.NoError(t, c.Heartbeat(context.Background(), beat(t, theirs)))

		require.Len(t, replyer.Replies(), 1)
		assert.Equal(t, "their-task", taskChanges(t, &replyer)[0].UUID)
	})

	t.Run("nothing is read from the runner while nobody is watching", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, _ := changes(t, &runner, &replyer)

		service := running()
		service.StackUUID = stackUUID

		require.NoError(t, c.Heartbeat(context.Background(), beat(t, service)))

		assert.Empty(t, replyer.Replies())
		runner.AssertNotCalled(t, "Stack", mock.Anything, mock.Anything)
	})

	t.Run("a service's beat carries its stack to the stack watches", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		runner.On("Stack", mock.Anything, stackUUID).Return(runnerManager.Stack{
			Stack:    runnerStack.Stack{UUID: stackUUID, Name: "blog", OwnerUUID: ownerUUID},
			State:    task.Running,
			Services: []task.Task{{UUID: taskUUID, Slug: "nginx-xkfqz"}},
		}, nil).Once()
		defer runner.AssertExpectations(t)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchStacks(everybodysWatch, "")

		service := running()
		service.StackUUID = stackUUID

		require.NoError(t, c.Heartbeat(context.Background(), beat(t, service)))

		replies := replyer.Replies()
		require.Len(t, replies, 1, "nobody is watching the tasks, so the stack is the only news")

		var change StackChange
		require.NoError(t, json.Unmarshal(replies[0].Payload, &change))

		assert.Equal(t, kindChanged, change.Kind)
		assert.Equal(t, stackUUID, change.UUID)
		require.NotNil(t, change.Stack)
		assert.Equal(t, "blog", change.Stack.Name)
		assert.Len(t, change.Stack.Services, 1)
	})

	t.Run("a malformed report is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchTasks(everybodysWatch, "")

		assert.NoError(t, c.Heartbeat(context.Background(), []byte("{")))
		assert.Empty(t, replyer.Replies())
	})
}

func TestChanges_Scheduled(t *testing.T) {
	t.Parallel()

	t.Run("a task appears before any node holds it", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchTasks(ownWatch, ownerUUID)

		payload, err := json.Marshal(taskEvents.TaskScheduled{
			UUID:          taskUUID,
			Name:          "nginx",
			OwnerUUID:     ownerUUID,
			Image:         "nginx:alpine",
			NominatedNode: "worker-a",
		})
		require.NoError(t, err)

		require.NoError(t, c.Scheduled(context.Background(), payload))

		require.Len(t, replyer.Replies(), 1)

		change := taskChanges(t, &replyer)[0]
		assert.Equal(t, kindChanged, change.Kind)
		require.NotNil(t, change.Task)
		assert.Equal(t, task.Scheduled.String(), change.Task.State)
	})
}

func TestChanges_Failed(t *testing.T) {
	t.Parallel()

	t.Run("a task that could not be run is reported, with why", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchTasks(ownWatch, ownerUUID)

		payload, err := json.Marshal(taskEvents.TaskFailed{
			UUID:      taskUUID,
			Name:      "nginx",
			OwnerUUID: ownerUUID,
			Reason:    "the image could not be pulled",
		})
		require.NoError(t, err)

		require.NoError(t, c.Failed(context.Background(), payload))

		require.Len(t, replyer.Replies(), 1)

		change := taskChanges(t, &replyer)[0]
		require.NotNil(t, change.Task)
		assert.Equal(t, task.Failed.String(), change.Task.State)
		assert.Equal(t, "the image could not be pulled", change.Task.Reason)
	})
}

func TestChanges_Deleted(t *testing.T) {
	t.Parallel()

	t.Run("a task that is gone is reported to every watch", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchTasks(everybodysWatch, "")
		watchers.WatchTasks(ownWatch, somebodyElse)

		payload, err := json.Marshal(taskEvents.TaskDeleted{UUID: taskUUID})
		require.NoError(t, err)

		require.NoError(t, c.Deleted(context.Background(), payload))

		replies := replyer.Replies()
		require.Len(t, replies, 2, "a task that is gone says only which one it was")

		for _, change := range taskChanges(t, &replyer) {
			assert.Equal(t, kindDeleted, change.Kind)
			assert.Equal(t, taskUUID, change.UUID)
			assert.Nil(t, change.Task)
		}
	})

	t.Run("a stack that is gone is reported to the stack watches", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		c, watchers := changes(t, &runner, &replyer)
		watchers.WatchStacks(everybodysWatch, "")

		payload, err := json.Marshal(stackEvents.StackDeleted{UUID: stackUUID})
		require.NoError(t, err)

		require.NoError(t, c.StackDeleted(context.Background(), payload))

		replies := replyer.Replies()
		require.Len(t, replies, 1)

		var change StackChange
		require.NoError(t, json.Unmarshal(replies[0].Payload, &change))

		assert.Equal(t, kindDeleted, change.Kind)
		assert.Equal(t, stackUUID, change.UUID)
		assert.Nil(t, change.Stack)
	})
}
