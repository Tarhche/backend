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

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/placement"
	runTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/runTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodeMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/nodes"
	stackMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/stacks"
	taskMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/scheduler/roundrobin"
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

// codes is a validator that refuses what a request says is wrong with itself,
// as the codes it says it with, so a test can read what was refused.
type codes struct{}

var _ domain.Validator = codes{}

func (codes) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

func healthyNodes(names ...string) []node.Node {
	nodes := make([]node.Node, len(names))
	for i, name := range names {
		nodes[i] = node.Node{Name: name, LastHeartbeatAt: time.Now()}
	}

	return nodes
}

func microVMs() runtime.Offer {
	return runtime.Offer{
		Class:   runtime.Firecracker,
		Driver:  "microvm",
		Healthy: true,
		Capabilities: runtime.Capabilities{
			Isolation:       runtime.IsolationMicroVM,
			NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
			StackNetworks:   true,
			ReadOnlyRoot:    true,
			DiskLimit:       true,
			TTY:             true,
			RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
		},
	}
}

func bothClasses(t *testing.T, defaultClass runtime.Class) allowed.Classes {
	t.Helper()

	classes, err := allowed.New([]runtime.Class{runtime.Sysbox, runtime.Firecracker}, defaultClass)
	require.NoError(t, err)

	return classes
}

func stackRequest(t *testing.T, body string) *Request {
	t.Helper()

	var request Request
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	request.OwnerUUID = "owner-uuid"

	return &request
}

// useCase is the use case over these doubles. The zero classes are sysbox
// alone, which is a platform configured as it always was.
func useCase(
	stacks *stackMocks.MockStacksRepository,
	nodes *nodeMocks.MockNodesRepository,
	tasks *taskMocks.MockTasksRepository,
	producer *messagingMock.MockProduceConsumer,
	v domain.Validator,
	classes allowed.Classes,
) *UseCase {
	return NewUseCase(
		stacks,
		runTask.NewUseCase(tasks, producer, v, classes),
		placement.New(nodes, roundrobin.New()),
		defaults,
		classes,
		v,
		discardLogger(),
	)
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
			Return(healthyNodes("workload-orchestrator-01", "workload-orchestrator-02"), nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		response, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, accepts(), allowed.Classes{}).
			Execute(context.Background(), stackRequest(t, twoServices))

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
			Return(healthyNodes("workload-orchestrator-01"), nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		_, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, accepts(), allowed.Classes{}).
			Execute(context.Background(), stackRequest(t, twoServices))

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
			Return(healthyNodes("workload-orchestrator-01"), nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		_, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, accepts(), allowed.Classes{}).
			Execute(context.Background(), stackRequest(t, `{
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

	t.Run("a stack no node can take right now is stood up all the same, and its services wait", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		// a node whose last heartbeat is old is passed over: it is being
		// redeployed, say, and it is coming back.
		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).
			Return([]node.Node{{Name: "workload-orchestrator-01", LastHeartbeatAt: time.Now().Add(-time.Hour)}}, nil).Once()

		var stored stack.Stack
		stackRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { stored = *args.Get(1).(*stack.Stack) }).
			Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		response, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, accepts(), allowed.Classes{}).
			Execute(context.Background(), stackRequest(t, twoServices))

		require.NoError(t, err)
		assert.Equal(t, "stack-uuid", response.UUID)

		// on no node yet: the first of its services to be placed decides
		// for the rest, as soon as a node can take it.
		assert.Empty(t, stored.NodeName)
		require.Len(t, created, 2)
		assert.Empty(t, created[0].NodeName)
		assert.Empty(t, created[1].NodeName)
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

		response, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, v, allowed.Classes{}).
			Execute(context.Background(), stackRequest(t, `{"name": "myapp"}`))

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
			Return(healthyNodes("workload-orchestrator-01"), nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("", unreachable).Once()

		_, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, accepts(), allowed.Classes{}).
			Execute(context.Background(), stackRequest(t, twoServices))

		assert.ErrorIs(t, err, unreachable)
		taskRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})
}

func TestUseCase_Execute_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a stack is run with the class it names, as is every service of it", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).Return([]node.Node{
			{Name: "workload-orchestrator-01", LastHeartbeatAt: time.Now()},
			{Name: "workload-orchestrator-02", Runtimes: []runtime.Offer{microVMs()}, LastHeartbeatAt: time.Now()},
		}, nil).Once()

		var stored stack.Stack
		stackRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { stored = *args.Get(1).(*stack.Stack) }).
			Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		response, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, codes{}, bothClasses(t, runtime.Sysbox)).
			Execute(context.Background(), stackRequest(t, `{
				"name": "myapp",
				"runtime": "firecracker",
				"services": {"web": {"image": "nginx:alpine"}, "db": {"image": "postgres:17", "runtime": "firecracker"}}
			}`))

		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		assert.Equal(t, runtime.Firecracker, stored.Runtime)
		assert.Equal(t, "workload-orchestrator-02", stored.NodeName, "the one node that offers the class")

		require.Len(t, created, 2)
		for _, service := range created {
			assert.Equal(t, runtime.Firecracker, service.Runtime)
			assert.Equal(t, "workload-orchestrator-02", service.NodeName)
		}
	})

	t.Run("a stack that names no class is the platform's default", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).
			Return(healthyNodes("workload-orchestrator-01"), nil).Once()

		var stored stack.Stack
		stackRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { stored = *args.Get(1).(*stack.Stack) }).
			Return("stack-uuid", nil).Once()

		var created []task.Task
		taskRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = append(created, *args.Get(1).(*task.Task)) }).
			Return("task-uuid", nil).Once()
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		_, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, codes{}, bothClasses(t, runtime.Sysbox)).
			Execute(context.Background(), stackRequest(t, `{"name": "myapp", "services": {"web": {"image": "nginx:alpine"}}}`))

		require.NoError(t, err)

		assert.Equal(t, runtime.Sysbox, stored.Runtime)
		require.Len(t, created, 1)
		assert.Equal(t, runtime.Sysbox, created[0].Runtime)
	})

	t.Run("a class the platform does not allow is refused, and nothing is created", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		response, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, codes{}, allowed.Classes{}).
			Execute(context.Background(), stackRequest(t, `{"name": "myapp", "runtime": "firecracker", "services": {"web": {"image": "nginx:alpine"}}}`))

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"runtime": "invalid_value"}, response.ValidationErrors)

		nodeRepository.AssertNotCalled(t, "GetAll", mock.Anything, mock.Anything, mock.Anything)
		stackRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("services that end up with two classes once the default is known are refused", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		// one names firecracker, the other nothing, which is sysbox here.
		response, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, codes{}, bothClasses(t, runtime.Sysbox)).
			Execute(context.Background(), stackRequest(t, `{
				"name": "myapp",
				"services": {"web": {"image": "nginx:alpine", "runtime": "firecracker"}, "db": {"image": "postgres:17"}}
			}`))

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"runtime": "mixed_runtimes_in_stack"}, response.ValidationErrors)

		stackRepository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("and the same services are one stack where firecracker is the default", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).Return([]node.Node{
			{Name: "workload-orchestrator-01", Runtimes: []runtime.Offer{microVMs()}, LastHeartbeatAt: time.Now()},
		}, nil).Once()
		stackRepository.On("Save", mock.Anything, mock.Anything).Return("stack-uuid", nil).Once()
		taskRepository.On("Save", mock.Anything, mock.Anything).Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		response, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, codes{}, bothClasses(t, runtime.Firecracker)).
			Execute(context.Background(), stackRequest(t, `{
				"name": "myapp",
				"services": {"web": {"image": "nginx:alpine", "runtime": "firecracker"}, "db": {"image": "postgres:17"}}
			}`))

		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)
	})

	t.Run("a node that cannot run every service of the stack is passed over", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		writable := microVMs()
		writable.Capabilities.ReadOnlyRoot = false

		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).Return([]node.Node{
			{Name: "workload-orchestrator-01", Runtimes: []runtime.Offer{writable}, LastHeartbeatAt: time.Now()},
			{Name: "workload-orchestrator-02", Runtimes: []runtime.Offer{microVMs()}, LastHeartbeatAt: time.Now()},
		}, nil).Once()

		var stored stack.Stack
		stackRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { stored = *args.Get(1).(*stack.Stack) }).
			Return("stack-uuid", nil).Once()
		taskRepository.On("Save", mock.Anything, mock.Anything).Return("task-uuid", nil).Times(2)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		_, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, codes{}, bothClasses(t, runtime.Firecracker)).
			Execute(context.Background(), stackRequest(t, `{
				"name": "myapp",
				"services": {"web": {"image": "nginx:alpine"}, "db": {"image": "postgres:17", "read_only": true}}
			}`))

		require.NoError(t, err)
		assert.Equal(t, "workload-orchestrator-02", stored.NodeName)
	})

	t.Run("a stack of a class no node offers is stood up, for its services to be failed as they are placed", func(t *testing.T) {
		t.Parallel()

		var (
			stackRepository stackMocks.MockStacksRepository
			nodeRepository  nodeMocks.MockNodesRepository
			taskRepository  taskMocks.MockTasksRepository
			producer        messagingMock.MockProduceConsumer
		)

		// sysbox alone, as every node said before there were classes.
		nodeRepository.On("GetAll", mock.Anything, mock.Anything, mock.Anything).
			Return(healthyNodes("workload-orchestrator-01"), nil).Once()

		var stored stack.Stack
		stackRepository.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { stored = *args.Get(1).(*stack.Stack) }).
			Return("stack-uuid", nil).Once()
		taskRepository.On("Save", mock.Anything, mock.Anything).Return("task-uuid", nil).Once()
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		_, err := useCase(&stackRepository, &nodeRepository, &taskRepository, &producer, codes{}, bothClasses(t, runtime.Sysbox)).
			Execute(context.Background(), stackRequest(t, `{"name": "myapp", "runtime": "firecracker", "services": {"web": {"image": "nginx:alpine"}}}`))

		require.NoError(t, err)
		assert.Equal(t, runtime.Firecracker, stored.Runtime)
		assert.Empty(t, stored.NodeName)
	})
}

// stack.Stack is what the repository double stores, so the compiler holds the
// double to the contract the use case depends on.
var _ stack.Repository = &stackMocks.MockStacksRepository{}
