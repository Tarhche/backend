package runTask

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/containers"
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

const nodeName = "runner-worker-01"

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("publishes every exposed port on a host port docker picks", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		var created *container.Container
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).Return([]container.Container{}, nil).Maybe()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*container.Container) }).
			Return("container-id", nil).Once()
		containerManager.On("Start", mock.Anything, "container-id").Return(nil).Once()
		defer containerManager.AssertExpectations(t)

		response, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.ExposedPorts = []port.Port{80, 443}
			}))

		require.NoError(t, err)
		assert.Equal(t, "container-id", response.UUID)

		require.NotNil(t, created)

		// an unset host port is docker's own "pick a free one", which is what
		// keeps the runner out of the business of tracking what is taken.
		require.Len(t, created.PortBindings, 2)
		for _, containerPort := range []port.Port{80, 443} {
			bindings := created.PortBindings[containerPort]
			require.Len(t, bindings, 1)
			assert.Zero(t, bindings[0].HostPort, "the host port is docker's to choose")
		}

		assert.Equal(t, port.PortSet{80: {}, 443: {}}, created.ExposedPorts)
	})

	t.Run("names the container by its slug, so it is called the same thing everywhere", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()

		var created *container.Container
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).Return([]container.Container{}, nil).Maybe()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*container.Container) }).
			Return("container-id", nil).Once()
		containerManager.On("Start", mock.Anything, "container-id").Return(nil).Once()

		_, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))

		require.NoError(t, err)

		assert.Equal(t, "nginx-xkfqz", created.Name)
		assert.Equal(t, "nginx-xkfqz", created.Labels[container.TaskSlugLabelKey])
		assert.Equal(t, "task-uuid", created.Labels[container.TaskUUIDLabelKey])
		assert.Equal(t, string(task.KindService), created.Labels[container.TaskKindLabelKey])
		assert.Equal(t, nodeName, created.Labels[container.NodeNameLabelKey])

		// kept false, so the container's logs and stats survive it exiting.
		assert.False(t, created.AutoRemove)
	})

	t.Run("a service joins its stack's network under its service name", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		networkManager.On("EnsureStackNetwork", mock.Anything, "myapp-abcde").Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		var created *container.Container
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).Return([]container.Container{}, nil).Maybe()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*container.Container) }).
			Return("container-id", nil).Once()
		containerManager.On("Start", mock.Anything, "container-id").Return(nil).Once()

		_, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.StackUUID = "stack-uuid"
				r.StackSlug = "myapp-abcde"
				r.ServiceName = "api"
			}))

		require.NoError(t, err)

		assert.Equal(t, []network.Attachment{
			{Name: "runner-stack-myapp-abcde", Aliases: []string{"api"}},
		}, created.Networks)
	})

	t.Run("a public container also joins the bridge, which is what routes out", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()

		var created *container.Container
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).Return([]container.Container{}, nil).Maybe()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*container.Container) }).
			Return("container-id", nil).Once()
		containerManager.On("Start", mock.Anything, "container-id").Return(nil).Once()

		_, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
			Execute(context.Background(), validRequest(func(r *Request) {
				r.NetworkPolicy = network.PolicyPublic
			}))

		require.NoError(t, err)

		assert.Equal(t, []network.Attachment{
			{Name: network.IsolatedNetworkName},
			{Name: network.PublicNetworkName, Gateway: true},
		}, created.Networks)
	})

	t.Run("a container with no network needs none made for it", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		var created *container.Container
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).Return([]container.Container{}, nil).Maybe()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*container.Container) }).
			Return("container-id", nil).Once()
		containerManager.On("Start", mock.Anything, "container-id").Return(nil).Once()

		_, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
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
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		refusal := domain.ValidationErrors{"exposed_ports": "ports_require_network"}

		response, err := NewUseCase(&containerManager, &networkManager, refuses(refusal), nodeName).
			Execute(context.Background(), validRequest(nil))

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)

		containerManager.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("a container that cannot be created is not started", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		expected := errors.New("the daemon is unreachable")

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).Return([]container.Container{}, nil).Maybe()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).Return("", expected).Once()

		// nothing was created, so there is nothing to take instead.
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).
			Return([]container.Container{}, nil).Once()

		_, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))

		assert.ErrorIs(t, err, expected)
		containerManager.AssertNotCalled(t, "Start", mock.Anything, mock.Anything)
	})

	t.Run("a container this task already has is taken rather than made twice", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		// what docker says when the same run is asked for twice, which is what
		// a message handed over again looks like from here.
		conflict := errors.New(`Conflict. The container name "/a-name" is already in use`)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		// looked at twice: once before anything is made, and once when making
		// it turns out to be unnecessary.
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).
			Return([]container.Container{{ID: "container-id"}}, nil).Twice()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).Return("", conflict).Once()
		containerManager.On("Start", mock.Anything, "container-id").Return(nil).Once()
		defer containerManager.AssertExpectations(t)

		response, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))

		require.NoError(t, err)
		assert.Equal(t, "container-id", response.UUID)
	})

	t.Run("a network that cannot be made stops the container being created", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		expected := errors.New("the network cannot be created")
		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(expected).Once()

		_, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
			Execute(context.Background(), validRequest(nil))

		assert.ErrorIs(t, err, expected)
		containerManager.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
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

	t.Run("a container with no slug falls back to its name", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "nginx", (&Request{Name: "nginx"}).ContainerName())
		assert.Equal(t, "nginx-xkfqz", (&Request{Name: "nginx", Slug: "nginx-xkfqz"}).ContainerName())
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
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		// the container that failed is still there, holding the name and the
		// ports the next attempt needs.
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Return([]container.Container{{ID: "failed-container-id"}}, nil).Once()
		containerManager.On("Delete", mock.Anything, "failed-container-id").Return(nil).Once()

		var created *container.Container
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, mock.Anything).Return([]container.Container{}, nil).Maybe()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*container.Container) }).
			Return("container-id", nil).Once()
		containerManager.On("Start", mock.Anything, "container-id").Return(nil).Once()
		defer containerManager.AssertExpectations(t)

		useCase := NewUseCase(&containerManager, &networkManager, accepts(), nodeName)

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
		assert.Equal(t, "container-id", response.UUID)

		// and the new one says which attempt it is, so that whoever reports on
		// it reports the failures behind it too.
		assert.Equal(t, "2", created.Labels[container.TaskAttemptLabelKey])
	})

	t.Run("a first attempt takes the container that is already there", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		// asked for twice, and the first attempt got as far as making one: it is
		// the attempt that was asked for, so it is started rather than replaced.
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Return([]container.Container{{ID: "existing-container-id"}}, nil).Twice()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).Return("", errors.New("name is already in use")).Once()
		containerManager.On("Start", mock.Anything, "existing-container-id").Return(nil).Once()
		defer containerManager.AssertExpectations(t)

		useCase := NewUseCase(&containerManager, &networkManager, accepts(), nodeName)

		response, err := useCase.Execute(context.Background(), &Request{
			UUID:  "task-uuid",
			Name:  "api",
			Slug:  "api-abcde",
			Kind:  task.KindService,
			Image: "nginx",
		})

		require.NoError(t, err)
		assert.Equal(t, "existing-container-id", response.UUID)

		containerManager.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	})
}

func TestUseCase_Execute_adopting(t *testing.T) {
	t.Parallel()

	t.Run("a container that is still standing is taken, whatever attempt it is", func(t *testing.T) {
		t.Parallel()

		var (
			containerManager containers.MockContainerManager
			networkManager   containers.MockNetworkManager
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		defer networkManager.AssertExpectations(t)

		// a node that was away for a while is asked for its containers again,
		// from the beginning, and they are still running.
		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Return([]container.Container{{ID: "running-container-id", Status: container.StatusRunning}}, nil).Twice()
		containerManager.On("EnsureImage", mock.Anything, mock.Anything).Once().Return(nil)
		containerManager.On("Create", mock.Anything, mock.Anything).Return("", errors.New("name is already in use")).Once()
		containerManager.On("Start", mock.Anything, "running-container-id").Return(nil).Once()
		defer containerManager.AssertExpectations(t)

		_, err := NewUseCase(&containerManager, &networkManager, accepts(), nodeName).
			Execute(context.Background(), &Request{
				UUID:  "task-uuid",
				Name:  "api",
				Slug:  "api-abcde",
				Kind:  task.KindService,
				Image: "nginx",
			})

		require.NoError(t, err)

		containerManager.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	})
}
