package runTask

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	"github.com/khanzadimahdi/testproject/application/workload/spec"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
)

// codes is a validator that refuses what a request says is wrong with itself,
// as the codes it says it with, so a test can read what was refused.
type codes struct{}

var _ domain.Validator = codes{}

func (codes) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

func bothClasses(t *testing.T, defaultClass runtime.Class) allowed.Classes {
	t.Helper()

	classes, err := allowed.New([]runtime.Class{runtime.Sysbox, runtime.Firecracker}, defaultClass)
	require.NoError(t, err)

	return classes
}

// runnable is a request with everything a task needs but its class.
func runnable() *Request {
	return &Request{
		Name:           "web",
		Kind:           task.KindService,
		Image:          "nginx:alpine",
		ResourceLimits: ResourceLimits{Cpu: 0.5, Memory: 128 << 20, Disk: 200 << 20},
		OwnerUUID:      "owner-uuid",
	}
}

func TestUseCase_Execute_Runtime(t *testing.T) {
	t.Parallel()

	stored := func(t *testing.T, classes allowed.Classes, request *Request) task.Task {
		t.Helper()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		var saved task.Task
		tasks.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { saved = *args.Get(1).(*task.Task) }).
			Return("task-uuid", nil).Once()
		defer tasks.AssertExpectations(t)

		producer.On("Produce", mock.Anything, events.TaskCreatedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		response, err := NewUseCase(&tasks, &producer, codes{}, classes).Execute(context.Background(), request)
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		return saved
	}

	t.Run("a task naming no class is stored with the default", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, runtime.Firecracker, stored(t, bothClasses(t, runtime.Firecracker), runnable()).Runtime)
	})

	t.Run("a task naming a class is stored with it", func(t *testing.T) {
		t.Parallel()

		request := runnable()
		request.Runtime = runtime.Sysbox

		assert.Equal(t, runtime.Sysbox, stored(t, bothClasses(t, runtime.Firecracker), request).Runtime)
	})

	t.Run("a platform configured as it always was runs sysbox", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, runtime.Sysbox, stored(t, allowed.Classes{}, runnable()).Runtime)
	})

	t.Run("a class the platform does not allow is refused, and nothing is stored", func(t *testing.T) {
		t.Parallel()

		testCases := map[string]runtime.Class{
			"a class it does not offer":   runtime.Firecracker,
			"a class nobody ever offered": "gvisor",
			"what cannot be a class":      "Fire Cracker",
		}

		for name, class := range testCases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				var (
					tasks    tasksMock.MockTasksRepository
					producer messagingMock.MockProduceConsumer
				)

				request := runnable()
				request.Runtime = class

				response, err := NewUseCase(&tasks, &producer, codes{}, allowed.Classes{}).Execute(context.Background(), request)
				require.NoError(t, err)

				assert.Equal(t, domain.ValidationErrors{"runtime": "invalid_value"}, response.ValidationErrors)
				tasks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("a compose service names its class under compose's own key", func(t *testing.T) {
		t.Parallel()

		var service spec.Service
		require.NoError(t, json.Unmarshal([]byte(`{"image": "nginx:alpine", "runtime": "firecracker"}`), &service))

		request := FromSpec("web", &service, task.ResourceLimits{Cpu: 0.5, Memory: 128 << 20, Disk: 200 << 20})
		request.OwnerUUID = "owner-uuid"

		assert.Equal(t, runtime.Firecracker, request.Runtime)
		assert.Equal(t, runtime.Firecracker, stored(t, bothClasses(t, runtime.Sysbox), request).Runtime)
	})
}

func TestTaskRunRequested_Handle(t *testing.T) {
	t.Parallel()

	requested := func(t *testing.T, event events.TaskRunRequested) []byte {
		t.Helper()

		payload, err := json.Marshal(event)
		require.NoError(t, err)

		return payload
	}

	codeRun := events.TaskRunRequested{
		Name:           "code-request-id",
		Kind:           string(task.KindJob),
		Runtime:        runtime.Firecracker,
		Image:          "ghcr.io/tarhche/code-runner:go-1.24-latest",
		ResourceLimits: events.ResourceLimits{Cpu: 2, Memory: 200 << 20, Disk: 100 << 20},
		OwnerUUID:      "guest",
	}

	t.Run("a snippet is run with the class the code runner asks for", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		var saved task.Task
		tasks.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { saved = *args.Get(1).(*task.Task) }).
			Return("task-uuid", nil).Once()
		producer.On("Produce", mock.Anything, events.TaskCreatedName, mock.Anything).Return(nil).Once()

		handler := NewTaskRunRequested(NewUseCase(&tasks, &producer, codes{}, bothClasses(t, runtime.Sysbox)), discardLogger())

		require.NoError(t, handler.Handle(context.Background(), requested(t, codeRun)))
		assert.Equal(t, runtime.Firecracker, saved.Runtime)
	})

	t.Run("one that cannot be stored is reported, rather than read for a refusal it never made", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		unreachable := errors.New("the database is unreachable")
		tasks.On("Save", mock.Anything, mock.Anything).Return("", unreachable).Once()

		handler := NewTaskRunRequested(NewUseCase(&tasks, &producer, codes{}, bothClasses(t, runtime.Sysbox)), discardLogger())

		assert.ErrorIs(t, handler.Handle(context.Background(), requested(t, codeRun)), unreachable)
	})

	t.Run("one asking for a class the platform does not allow is refused, which is not an error", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		handler := NewTaskRunRequested(NewUseCase(&tasks, &producer, codes{}, allowed.Classes{}), discardLogger())

		require.NoError(t, handler.Handle(context.Background(), requested(t, codeRun)))
		tasks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})
}
