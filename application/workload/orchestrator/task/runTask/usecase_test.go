package runTask

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	workloadRuntime "github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	driverMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/driver"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

// accepts is a validator that passes whatever it is given, so a use case test
// is about what the use case does rather than about the rules, which
// request_test.go covers.
func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

// refuses is a validator that rejects whatever it is given.
func refuses(validationErrors domain.ValidationErrors) *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(validationErrors)

	return v
}

const nodeName = "workload-orchestrator-01"

// classDriver is the driver of a class, whose runs are tasks and whose
// networks are networks.
func classDriver(class workloadRuntime.Class, tasks task.Runtime, networks network.Manager) *driverMock.MockDriver {
	d := &driverMock.MockDriver{}
	d.On("Class").Return(class).Maybe()
	d.On("Tasks").Return(tasks).Maybe()
	d.On("Networks").Return(networks).Maybe()

	return d
}

// node is a node offering the drivers it is given, and no other class.
func node(drivers ...*driverMock.MockDriver) *driverMock.MockSet {
	set := &driverMock.MockSet{}

	for _, d := range drivers {
		set.On("For", d.Class()).Return(d, nil).Maybe()
	}

	set.On("For", mock.Anything).Return(nil, driver.ErrUnknownClass).Maybe()

	return set
}

// sysboxOnly is a node that offers sysbox alone, as every node did before
// there were classes.
func sysboxOnly(tasks task.Runtime, networks network.Manager) *driverMock.MockSet {
	return node(classDriver(workloadRuntime.Sysbox, tasks, networks))
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("publishes every exposed port on a host port docker picks", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		var created *task.Execution
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return("task-id", nil).Once()
		taskManager.On("Start", mock.Anything, "task-id").Return(nil).Once()
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.ExposedPorts = []port.Port{80, 443}
			}))

		require.NoError(t, err)
		assert.Equal(t, "task-id", response.UUID)

		require.NotNil(t, created)

		// an unset host port is docker's own "pick a free one", which is what
		// keeps the workload out of the business of tracking what is taken.
		require.Len(t, created.PortBindings, 2)
		for _, taskPort := range []port.Port{80, 443} {
			bindings := created.PortBindings[taskPort]
			require.Len(t, bindings, 1)
			assert.Zero(t, bindings[0].HostPort, "the host port is docker's to choose")
		}

		assert.Equal(t, port.PortSet{80: {}, 443: {}}, created.ExposedPorts)
	})

	t.Run("names the task by its slug, so it is called the same thing everywhere", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()

		var created *task.Execution
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return("task-id", nil).Once()
		taskManager.On("Start", mock.Anything, "task-id").Return(nil).Once()

		_, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))

		require.NoError(t, err)

		assert.Equal(t, "nginx-xkfqz", created.Name)
		assert.Equal(t, "nginx-xkfqz", created.Slug)
		assert.Equal(t, "task-uuid", created.TaskUUID)
		assert.Equal(t, task.KindService, created.Kind)
		assert.Equal(t, nodeName, created.NodeName)

		// kept false, so the task's logs and stats survive it exiting.
		assert.False(t, created.AutoRemove)
	})

	t.Run("a service joins its stack's network under its service name", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		networkManager.On("EnsureStackNetwork", mock.Anything, "myapp-abcde").Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		var created *task.Execution
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return("task-id", nil).Once()
		taskManager.On("Start", mock.Anything, "task-id").Return(nil).Once()

		_, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.StackUUID = "stack-uuid"
				r.StackSlug = "myapp-abcde"
				r.ServiceName = "api"
			}))

		require.NoError(t, err)

		assert.Equal(t, []network.Attachment{
			{Name: "workload-stack-myapp-abcde", Aliases: []string{"api"}},
		}, created.Networks)
	})

	t.Run("a public task also joins the bridge, which is what routes out", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()

		var created *task.Execution
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return("task-id", nil).Once()
		taskManager.On("Start", mock.Anything, "task-id").Return(nil).Once()

		_, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.NetworkPolicy = network.PolicyPublic
			}))

		require.NoError(t, err)

		assert.Equal(t, []network.Attachment{
			{Name: network.IsolatedNetworkName},
			{Name: network.PublicNetworkName, Gateway: true},
		}, created.Networks)
	})

	t.Run("a task with no network needs none made for it", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		var created *task.Execution
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return("task-id", nil).Once()
		taskManager.On("Start", mock.Anything, "task-id").Return(nil).Once()

		_, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.NetworkPolicy = network.PolicyNone
				r.ExposedPorts = nil
			}))

		require.NoError(t, err)

		assert.Equal(t, []network.Attachment{{Name: network.NoNetworkName}}, created.Networks)
		networkManager.AssertNotCalled(t, "EnsureIsolatedNetwork", mock.Anything)
	})

	t.Run("a request the rules refuse never reaches the daemon", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		refusal := domain.ValidationErrors{"exposed_ports": "ports_require_network"}

		response, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), refuses(refusal), nodeName).
			Execute(context.Background(), validRequest(nil))

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)

		taskManager.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("a task that cannot be created is not started", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		expected := errors.New("the daemon is unreachable")

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).Return("", expected).Once()

		// nothing was created, so there is nothing to take instead.
		taskManager.On("Of", mock.Anything, mock.Anything).
			Return([]task.Execution{}, nil).Once()

		_, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))

		assert.ErrorIs(t, err, expected)
		taskManager.AssertNotCalled(t, "Start", mock.Anything, mock.Anything)
	})

	t.Run("a task this task already has is taken rather than made twice", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		// what docker says when the same run is asked for twice, which is what
		// a message handed over again looks like from here.
		conflict := errors.New(`Conflict. The task name "/a-name" is already in use`)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		// looked at twice: once before anything is made, and once when making
		// it turns out to be unnecessary.
		taskManager.On("Of", mock.Anything, mock.Anything).
			Return([]task.Execution{{ID: "task-id"}}, nil).Twice()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).Return("", conflict).Once()
		taskManager.On("Start", mock.Anything, "task-id").Return(nil).Once()
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))

		require.NoError(t, err)
		assert.Equal(t, "task-id", response.UUID)
	})

	t.Run("a network that cannot be made stops the task being created", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		expected := errors.New("the network cannot be created")
		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(expected).Once()

		_, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))

		assert.ErrorIs(t, err, expected)
		taskManager.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})
}

func TestRequest_Policy(t *testing.T) {
	t.Parallel()

	t.Run("a request naming no policy runs under the safe default", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, network.DefaultPolicy, (&Request{}).Policy())
		assert.Equal(t, network.PolicyIsolated, (&Request{}).Policy())
	})

	t.Run("a request naming no kind is a job, which is what every task was", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, task.KindJob, (&Request{}).TaskKind())
	})

	t.Run("a task with no slug falls back to its name", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "nginx", (&Request{Name: "nginx"}).TaskName())
		assert.Equal(t, "nginx-xkfqz", (&Request{Name: "nginx", Slug: "nginx-xkfqz"}).TaskName())
	})
}

// validRequest is a request that passes validation, with room to change the one
// thing a test is about.
func validRequest(change func(*Request)) *Request {
	r := &Request{
		UUID:          "task-uuid",
		Name:          "nginx",
		Slug:          "nginx-xkfqz",
		Kind:          task.KindService,
		Image:         "nginx:alpine",
		ExposedPorts:  []port.Port{80},
		NetworkPolicy: network.PolicyIsolated,
		ResourceLimits: ResourceLimits{
			Cpu:    0.5,
			Memory: 256 << 20,
			Disk:   1 << 30,
		},
	}

	if change != nil {
		change(r)
	}

	return r
}

func TestUseCase_Execute_retrying(t *testing.T) {
	t.Parallel()

	t.Run("an attempt after a failure replaces what is left of the last one", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		// the task that failed is still there, holding the name and the
		// ports the next attempt needs.
		taskManager.On("Of", mock.Anything, "task-uuid").
			Return([]task.Execution{{ID: "failed-task-id"}}, nil).Once()
		taskManager.On("Delete", mock.Anything, "failed-task-id").Return(nil).Once()

		var created *task.Execution
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return("task-id", nil).Once()
		taskManager.On("Start", mock.Anything, "task-id").Return(nil).Once()
		defer taskManager.AssertExpectations(t)

		useCase := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName)

		response, err := useCase.Execute(context.Background(), &Request{
			UUID:       "task-uuid",
			Name:       "api",
			Slug:       "api-abcde",
			Kind:       task.KindService,
			Image:      "nginx",
			Attempt:    2,
			MaxRetries: 3,
		})

		require.NoError(t, err)
		assert.Equal(t, "task-id", response.UUID)

		// and the new one says which attempt it is, so that whoever reports on
		// it reports the failures behind it too.
		assert.Equal(t, 2, created.Attempt)
	})

	t.Run("a first attempt takes the task that is already there", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		// asked for twice, and the first attempt got as far as making one: it is
		// the attempt that was asked for, so it is started rather than replaced.
		taskManager.On("Of", mock.Anything, "task-uuid").
			Return([]task.Execution{{ID: "existing-task-id"}}, nil).Twice()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).Return("", errors.New("name is already in use")).Once()
		taskManager.On("Start", mock.Anything, "existing-task-id").Return(nil).Once()
		defer taskManager.AssertExpectations(t)

		useCase := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName)

		response, err := useCase.Execute(context.Background(), &Request{
			UUID:  "task-uuid",
			Name:  "api",
			Slug:  "api-abcde",
			Kind:  task.KindService,
			Image: "nginx",
		})

		require.NoError(t, err)
		assert.Equal(t, "existing-task-id", response.UUID)

		taskManager.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	})
}

func TestUseCase_Execute_adopting(t *testing.T) {
	t.Parallel()

	t.Run("a task that is still standing is taken, whatever attempt it is", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		// a node that was away for a while is asked for its tasks again,
		// from the beginning, and they are still running.
		taskManager.On("Of", mock.Anything, "task-uuid").
			Return([]task.Execution{{ID: "running-task-id", Status: task.StatusRunning}}, nil).Twice()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).Return("", errors.New("name is already in use")).Once()
		taskManager.On("Start", mock.Anything, "running-task-id").Return(nil).Once()
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), &Request{
				UUID:  "task-uuid",
				Name:  "api",
				Slug:  "api-abcde",
				Kind:  task.KindService,
				Image: "nginx",
			})

		require.NoError(t, err)

		taskManager.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	})
}

func TestUseCase_Execute_classes(t *testing.T) {
	t.Parallel()

	// vmID is what vmhost calls a VM.
	const vmID = "0123456789abcdef"

	t.Run("a task is run by the driver of its class, and named by it", func(t *testing.T) {
		t.Parallel()

		var (
			sysboxTasks         runtime.MockRuntime
			sysboxNetworks      runtime.MockNetworkManager
			firecrackerTasks    runtime.MockRuntime
			firecrackerNetworks runtime.MockNetworkManager
		)

		drivers := node(
			classDriver(workloadRuntime.Sysbox, &sysboxTasks, &sysboxNetworks),
			classDriver(workloadRuntime.Firecracker, &firecrackerTasks, &firecrackerNetworks),
		)

		firecrackerNetworks.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()

		var created *task.Execution
		firecrackerTasks.On("Of", mock.Anything, "task-uuid").Return([]task.Execution{}, nil).Once()
		firecrackerTasks.On("EnsureImage", mock.Anything, "nginx:alpine").Return(nil).Once()
		firecrackerTasks.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return(vmID, nil).Once()
		firecrackerTasks.On("Start", mock.Anything, vmID).Return(nil).Once()
		defer firecrackerTasks.AssertExpectations(t)
		defer firecrackerNetworks.AssertExpectations(t)

		response, err := NewUseCase(drivers, accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.Runtime = workloadRuntime.Firecracker
			}))
		require.NoError(t, err)

		// named the way the rest of the node names it.
		assert.Equal(t, "firecracker:"+vmID, response.UUID)
		assert.Equal(t, workloadRuntime.Firecracker, created.Runtime)

		sysboxTasks.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		sysboxNetworks.AssertNotCalled(t, "EnsureIsolatedNetwork", mock.Anything)
	})

	t.Run("a task that names no class is run as sysbox, as every task was", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()

		var created *task.Execution
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Return(nil).Once()
		taskManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return("4f2c9d0b7a1e", nil).Once()
		taskManager.On("Start", mock.Anything, "4f2c9d0b7a1e").Return(nil).Once()

		response, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))
		require.NoError(t, err)

		// a sysbox run keeps the bare name docker gave it.
		assert.Equal(t, "4f2c9d0b7a1e", response.UUID)
		assert.Equal(t, workloadRuntime.Sysbox, created.Runtime)
	})

	t.Run("a class this node does not offer is not run as anything else", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
		)

		_, err := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.Runtime = workloadRuntime.Firecracker
			}))

		assert.ErrorIs(t, err, driver.ErrUnknownClass)

		networkManager.AssertNotCalled(t, "EnsureIsolatedNetwork", mock.Anything)
		taskManager.AssertNotCalled(t, "EnsureImage", mock.Anything, mock.Anything)
		taskManager.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})
}
