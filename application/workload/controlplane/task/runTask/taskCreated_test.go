package runTask

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/schedule"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/nodes"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/scheduler/roundrobin"
)

func created(t *testing.T, uuid string) []byte {
	t.Helper()

	payload, err := json.Marshal(events.TaskCreated{UUID: uuid})
	require.NoError(t, err)

	return payload
}

func TestTaskCreated_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a task nominated nowhere goes wherever there is room", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		unplaced := task.Task{UUID: "task-uuid", CurrentState: task.Created}

		tasks.On("GetOne", mock.Anything, unplaced.UUID).Return(unplaced, nil).Once()
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.NodeName == "workload-orchestrator-02" && t.CurrentState == task.Scheduled
		})).Return(unplaced.UUID, nil).Once()
		defer tasks.AssertExpectations(t)

		nodes.On("GetAll", mock.Anything, uint(0), uint(nominatedNodesLimit)).
			Return([]node.Node{{Name: "workload-orchestrator-02", LastHeartbeatAt: time.Now()}}, nil).Once()
		defer nodes.AssertExpectations(t)

		var scheduled events.TaskScheduled
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &scheduled) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		handler := NewTaskCreated(&tasks, &nodes, roundrobin.New(), schedule.New(&producer), discardLogger())

		require.NoError(t, handler.Handle(context.Background(), created(t, unplaced.UUID)))

		assert.Equal(t, "workload-orchestrator-02", scheduled.NominatedNode)
	})

	t.Run("a task goes where it was nominated", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		standalone := task.Task{UUID: "task-uuid", CurrentState: task.Created, NodeName: "workload-orchestrator-03"}

		tasks.On("GetOne", mock.Anything, standalone.UUID).Return(standalone, nil).Once()
		tasks.On("Save", mock.Anything, mock.Anything).Return(standalone.UUID, nil).Once()

		var scheduled events.TaskScheduled
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &scheduled) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		handler := NewTaskCreated(&tasks, &nodes, roundrobin.New(), schedule.New(&producer), discardLogger())

		require.NoError(t, handler.Handle(context.Background(), created(t, standalone.UUID)))

		assert.Equal(t, "workload-orchestrator-03", scheduled.NominatedNode)

		// nothing was chosen: it had already been placed.
		nodes.AssertNotCalled(t, "GetAll", mock.Anything, mock.Anything, mock.Anything)
	})
}
