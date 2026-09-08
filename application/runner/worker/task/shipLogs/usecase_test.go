package shipLogs

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/containers"
)

const nodeName = "runner-worker-01"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// shipped collects the batches a worker sends, so a test can wait for lines to
// arrive rather than guess at timing.
type shipped struct {
	lock   sync.Mutex
	events []events.TaskLogged
}

func (s *shipped) record(payload []byte) {
	var event events.TaskLogged
	if err := json.Unmarshal(payload, &event); err != nil {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.events = append(s.events, event)
}

func (s *shipped) lines() []events.LogLine {
	s.lock.Lock()
	defer s.lock.Unlock()

	var all []events.LogLine
	for _, event := range s.events {
		all = append(all, event.Lines...)
	}

	return all
}

// serviceContainer is a long-running container as the daemon reports it.
func serviceContainer(id string, taskUUID string) container.Container {
	return container.Container{
		ID: id,
		Labels: map[string]string{
			container.NodeNameLabelKey: nodeName,
			container.TaskUUIDLabelKey: taskUUID,
			container.TaskKindLabelKey: string(task.KindService),
		},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("ships what a container wrote even when it then goes quiet", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			producer         messagingMock.MockProduceConsumer
			collected        shipped
		)

		containerManager.On("GetByLabel", mock.Anything, container.NodeNameLabelKey, nodeName).
			Return([]container.Container{serviceContainer("container-1", "task-1")}, nil)

		// a burst of output and then nothing more, which is exactly what a
		// server does: it says it has started and then waits for work.
		containerManager.On("StreamLogs", mock.Anything, "container-1", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				emit := args.Get(3).(func(container.LogLine) error)

				for _, content := range []string{"starting", "listening on :80"} {
					_ = emit(container.LogLine{
						Stream:  container.StreamStdout,
						Content: content,
						At:      time.Now(),
					})
				}

				// the follow stays open, the way it does against a container
				// that is still running.
				<-args.Get(0).(context.Context).Done()
			}).
			Return(nil).Maybe()

		producer.On("Produce", mock.Anything, events.TaskLoggedName, mock.Anything).
			Run(func(args mock.Arguments) { collected.record(args.Get(2).([]byte)) }).
			Return(nil).Maybe()

		useCase := NewUseCase(&containerManager, &producer, nodeName, discardLogger())
		defer useCase.Close()

		require.NoError(t, useCase.Execute(context.Background()))

		// the batch has a clock of its own, so a burst followed by silence is
		// sent rather than held back waiting for a line that never comes.
		require.Eventually(t, func() bool { return len(collected.lines()) == 2 }, 5*time.Second, 20*time.Millisecond)

		lines := collected.lines()
		assert.Equal(t, "starting", lines[0].Content)
		assert.Equal(t, "listening on :80", lines[1].Content)
		assert.Equal(t, uint8(container.StreamStdout), lines[0].Stream)
	})

	t.Run("follows a container once, however often it is asked", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			producer         messagingMock.MockProduceConsumer
		)

		var reads atomic.Int32

		containerManager.On("GetByLabel", mock.Anything, container.NodeNameLabelKey, nodeName).
			Return([]container.Container{serviceContainer("container-1", "task-1")}, nil)
		containerManager.On("StreamLogs", mock.Anything, "container-1", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				reads.Add(1)
				<-args.Get(0).(context.Context).Done()
			}).
			Return(nil).Maybe()
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

		useCase := NewUseCase(&containerManager, &producer, nodeName, discardLogger())
		defer useCase.Close()

		for range 3 {
			require.NoError(t, useCase.Execute(context.Background()))
		}

		// a container is registered before the goroutine reading it has run, so
		// the read itself is what has to be waited for.
		require.Eventually(t, func() bool { return reads.Load() == 1 }, 2*time.Second, 10*time.Millisecond)

		// and it stays at one, however often the container is seen again.
		require.NoError(t, useCase.Execute(context.Background()))
		time.Sleep(100 * time.Millisecond)

		assert.Equal(t, int32(1), reads.Load(), "a container is followed once, however often it is seen")
		assert.Equal(t, 1, useCase.following())
	})

	t.Run("lets go of a container that is no longer there", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			producer         messagingMock.MockProduceConsumer
		)

		// present at first, gone by the second look.
		containerManager.On("GetByLabel", mock.Anything, container.NodeNameLabelKey, nodeName).
			Return([]container.Container{serviceContainer("container-1", "task-1")}, nil).Once()
		containerManager.On("GetByLabel", mock.Anything, container.NodeNameLabelKey, nodeName).
			Return([]container.Container{}, nil).Once()
		containerManager.On("StreamLogs", mock.Anything, "container-1", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { <-args.Get(0).(context.Context).Done() }).
			Return(nil).Maybe()
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

		useCase := NewUseCase(&containerManager, &producer, nodeName, discardLogger())
		defer useCase.Close()

		require.NoError(t, useCase.Execute(context.Background()))
		require.Eventually(t, func() bool { return useCase.following() == 1 }, 2*time.Second, 10*time.Millisecond)

		require.NoError(t, useCase.Execute(context.Background()))

		assert.Eventually(t, func() bool { return useCase.following() == 0 }, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("a one-shot job's log rides its heartbeat, so it is not followed", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			producer         messagingMock.MockProduceConsumer
		)

		job := serviceContainer("container-1", "task-1")
		job.Labels[container.TaskKindLabelKey] = string(task.KindJob)

		containerManager.On("GetByLabel", mock.Anything, container.NodeNameLabelKey, nodeName).
			Return([]container.Container{job}, nil)

		useCase := NewUseCase(&containerManager, &producer, nodeName, discardLogger())
		defer useCase.Close()

		require.NoError(t, useCase.Execute(context.Background()))

		assert.Equal(t, 0, useCase.following())
		containerManager.AssertNotCalled(t, "StreamLogs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}
