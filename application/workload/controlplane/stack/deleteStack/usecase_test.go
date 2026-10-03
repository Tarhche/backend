package deleteStack

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	deleteTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	stackEvents "github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
	stacksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/stacks"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	deleted := func(t *testing.T, s stack.Stack) stackEvents.StackDeleted {
		t.Helper()

		var (
			stacks     stacksMock.MockStacksRepository
			tasks      tasksMock.MockTasksRepository
			producer   messagingMock.MockProduceConsumer
			translator translator.TranslatorMock
		)

		stacks.On("GetOne", mock.Anything, s.UUID).Return(s, nil).Once()
		stacks.On("Delete", mock.Anything, s.UUID).Return(nil).Once()
		defer stacks.AssertExpectations(t)

		// a stack whose services are gone already: what is said about the
		// network it leaves behind is what this is about.
		tasks.On("GetAllByStack", mock.Anything, s.UUID).Return([]task.Task{}, nil).Once()

		var event stackEvents.StackDeleted
		producer.On("Produce", mock.Anything, stackEvents.StackDeletedName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &event) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		useCase := NewUseCase(
			&stacks,
			&tasks,
			deleteTask.NewUseCase(&tasks, logsMock.NewInMemoryRepository(), &producer, &translator),
			&producer,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		_, err := useCase.Execute(context.Background(), &Request{UUID: s.UUID})
		require.NoError(t, err)

		return event
	}

	t.Run("the network a stack leaves is taken away by the class that made it", func(t *testing.T) {
		t.Parallel()

		event := deleted(t, stack.Stack{
			UUID:     "stack-uuid",
			Slug:     "myapp-abcde",
			Runtime:  runtime.Firecracker,
			NodeName: "workload-orchestrator-01",
		})

		assert.Equal(t, "stack-uuid", event.UUID)
		assert.Equal(t, "myapp-abcde", event.Slug)
		assert.Equal(t, "workload-orchestrator-01", event.NodeName)
		assert.Equal(t, runtime.Firecracker, event.Runtime)
	})

	t.Run("a stack from before there were classes was sysbox's", func(t *testing.T) {
		t.Parallel()

		event := deleted(t, stack.Stack{UUID: "stack-uuid", Slug: "myapp-abcde", NodeName: "workload-orchestrator-01"})

		assert.Equal(t, runtime.Sysbox, event.Runtime)
	})
}
