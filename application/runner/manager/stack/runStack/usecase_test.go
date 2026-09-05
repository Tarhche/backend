package runStack

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	runTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/runTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodeMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/nodes"
	stackMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/stacks"
	taskMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/scheduler/roundrobin"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

var defaults = task.ResourceLimits{Cpu: 0.5, Memory: 128 << 20, Disk: 200 << 20}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func healthyNodes(names ...string) []node.Node {
	nodes := make([]node.Node, len(names))
	for i, name := range names {
		nodes[i] = node.Node{Name: name, LastHeartbeatAt: time.Now()}
	}

	return nodes
}

func stackRequest(t *testing.T, body string) *Request {
	t.Helper()

	var request Request
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	request.OwnerUUID = "owner-uuid"

	return &request
}

// dnsLabel is what a stack's slug has to be: it names the private network its
// services share.
var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	const twoServices = `{
		"name": "myapp",
		"services": {
			"web": {"image": "nginx:alpine", "ports": ["80"]},
			"api": {"image": "api:1", "environment": {"DATABASE_URL": "postgres://db:5432/app"}}
		}
	}`

	t.Run("places every service of a stack on one node", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).
			Return(healthyNodes("runner-worker-01", "runner-worker-02"), nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		response, err := NewUseCase(
			&stackRepository, &nodeRepository,
			runTask.NewUseCase(&taskRepository, &producer, accepts()),
			roundrobin.New(), defaults, accepts(), discardLogger(),
		).Execute(context.Background(), stackRequest(t, twoServices))

		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)
		require.Len(t, created, 2)

		// the services share a private network, and a bridge is local to the
		// node that made it, so they cannot be spread across nodes.
		assert.Equal(t, created[0].NodeName, created[1].NodeName)
		assert.NotEmpty(t, created[0].NodeName)
	})

	t.Run("names each service so its neighbours can reach it", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).
			Return(healthyNodes("runner-worker-01"), nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		_, err := NewUseCase(
			&stackRepository, &nodeRepository,
			runTask.NewUseCase(&taskRepository, &producer, accepts()),
			roundrobin.New(), defaults, accepts(), discardLogger(),
		).Execute(context.Background(), stackRequest(t, twoServices))

		require.NoError(t, err)
		require.Len(t, created, 2)

		// created in a settled order, so a failure part-way through is
		// reproducible.
		assert.Equal(t, "api", created[0].ServiceName)
		assert.Equal(t, "web", created[1].ServiceName)

		for _, service := range created {
			assert.Equal(t, "stack-uuid", service.StackUUID)
			assert.Equal(t, task.KindService, service.Kind)
			assert.True(t, strings.HasPrefix(service.Name, "myapp-"), "got %q", service.Name)
			assert.Regexp(t, dnsLabel, service.Slug)
		}
	})

	t.Run("fills in the limits a service did not name", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).
			Return(healthyNodes("runner-worker-01"), nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		_, err := NewUseCase(
			&stackRepository, &nodeRepository,
			runTask.NewUseCase(&taskRepository, &producer, accepts()),
			roundrobin.New(), defaults, accepts(), discardLogger(),
		).Execute(context.Background(), stackRequest(t, `{
			"name": "myapp",
			"services": {
				"lean": {"image": "a:1"},
				"fat":  {"image": "b:1", "deploy": {"resources": {"limits": {"cpus": "2", "memory": "1G"}}}}
			}
		}`))

		require.NoError(t, err)
		require.Len(t, created, 2)

		byName := map[string]task.Task{}
		for _, service := range created {
			byName[service.ServiceName] = service
		}

		assert.Equal(t, defaults, byName["lean"].ResourceLimits)
		assert.Equal(t, task.ResourceLimits{Cpu: 2, Memory: 1 << 30, Disk: defaults.Disk}, byName["fat"].ResourceLimits)

		// a service that named no policy runs under the safe default.
		assert.Equal(t, network.DefaultPolicy, byName["lean"].NetworkPolicy)
	})

	t.Run("a stack cannot be placed when no node is healthy", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		// a node whose last heartbeat is old is passed over.
		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).
			Return([]node.Node{{Name: "runner-worker-01", LastHeartbeatAt: time.Now().Add(-time.Hour)}}, nil).Once()

		_, err := NewUseCase(
			&stackRepository, &nodeRepository,
			runTask.NewUseCase(&taskRepository, &producer, accepts()),
			roundrobin.New(), defaults, accepts(), discardLogger(),
		).Execute(context.Background(), stackRequest(t, twoServices))

		assert.ErrorIs(t, err, node.ErrNoNodesAvailable)
		stackRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("a request the rules refuse creates nothing", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		refusal := domain.ValidationErrors{"services": "required_field"}

		v := &validator.MockValidator{}
		v.On("Validate", mock.Anything).Return(refusal)

		response, err := NewUseCase(
			&stackRepository, &nodeRepository,
			runTask.NewUseCase(&taskRepository, &producer, accepts()),
			roundrobin.New(), defaults, v, discardLogger(),
		).Execute(context.Background(), stackRequest(t, `{"name": "myapp"}`))

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)

		nodeRepository.AssertNotCalled(t, "GetAll", mock.Anything, mock.Anything, mock.Anything)
		stackRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("a stack that cannot be stored is reported", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		unreachable := errors.New("the database is unreachable")

		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).
			Return(healthyNodes("runner-worker-01"), nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("", unreachable).Once()

		_, err := NewUseCase(
			&stackRepository, &nodeRepository,
			runTask.NewUseCase(&taskRepository, &producer, accepts()),
			roundrobin.New(), defaults, accepts(), discardLogger(),
		).Execute(context.Background(), stackRequest(t, twoServices))

		assert.ErrorIs(t, err, unreachable)
		taskRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})
}

// stack.Stack is what the repository double stores, so the compiler holds the
// double to the contract the use case depends on.
var _ stack.Repository = &stackMocks.MockStacksRepository{}
