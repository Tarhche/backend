package runContainer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/manager"
	usersMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/manager/client"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

const ingressDomain = "runner.localhost:8021"

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

// composeRequest reads a compose service the way the dashboard's form sends it.
func composeRequest(t *testing.T, body string) *Request {
	t.Helper()

	var request Request
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	request.OwnerUUID = "owner-uuid"

	return &request
}

// directory answers about whoever a container belongs to, which for these is
// nobody in particular: what is presented is covered where presenting is.
func directory() *owners.Directory {
	users := &usersMock.MockUsersRepository{}
	users.On("GetByUUIDs", mock.Anything, mock.Anything).Return([]user.User{}, nil).Maybe()

	return owners.NewDirectory(users)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("hands the specification to the runner as it was written", func(t *testing.T) {
		t.Parallel()

		var runner manager.MockClient

		var handed runnerManager.ContainerSpec
		runner.On("RunContainer", mock.Anything, mock.Anything, "owner-uuid").
			Run(func(args mock.Arguments) { handed = args.Get(1).(runnerManager.ContainerSpec) }).
			Return(task.Task{
				UUID:         "task-uuid",
				Name:         "nginx",
				Slug:         "nginx-xkfqz",
				CurrentState: task.Created,
				Endpoints:    []task.Endpoint{{ContainerPort: 80}},
			}, nil).Once()
		defer runner.AssertExpectations(t)

		response, err := NewUseCase(&runner, accepts(), directory(), ingressDomain).Execute(
			context.Background(),
			composeRequest(t, `{
				"name": "nginx",
				"image": "nginx:1.27-alpine",
				"ports": ["8080:80"],
				"environment": {"TZ": "UTC"},
				"network_mode": "public"
			}`),
		)

		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		assert.Equal(t, "nginx", handed.Name)

		service, ok := handed.Service.(spec.Service)
		require.True(t, ok, "the specification travels in the shape it was written")
		assert.Equal(t, "nginx:1.27-alpine", service.Image)
		assert.Equal(t, []port.Port{80}, service.ExposedPorts())
		assert.Equal(t, network.PolicyPublic, service.NetworkPolicy())

		// answered with the addresses the container will be served on, so the
		// dashboard can link straight to it.
		require.NotNil(t, response.Container)
		assert.Equal(t, "nginx-xkfqz", response.Container.Slug)
		require.Len(t, response.Container.Endpoints, 1)
		assert.Equal(t, "http://nginx-xkfqz.runner.localhost:8021", response.Container.Endpoints[0].URL)
	})

	t.Run("what the runner refuses is reported as it stands", func(t *testing.T) {
		t.Parallel()

		var runner manager.MockClient

		// the runner decides what it can run, so its verdict reaches the person
		// who asked rather than being flattened into a failure.
		refusal := domain.ValidationErrors{"exposed_ports": "a container with no network cannot expose ports"}

		runner.On("RunContainer", mock.Anything, mock.Anything, "owner-uuid").
			Return(task.Task{}, &client.ValidationError{ValidationErrors: refusal}).Once()

		response, err := NewUseCase(&runner, accepts(), directory(), ingressDomain).Execute(
			context.Background(),
			composeRequest(t, `{"name": "nginx", "image": "nginx:alpine"}`),
		)

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)
		assert.Nil(t, response.Container)
	})

	t.Run("a runner that cannot be reached is a failure, not a refusal", func(t *testing.T) {
		t.Parallel()

		var runner manager.MockClient

		unreachable := errors.New("the runner is unreachable")
		runner.On("RunContainer", mock.Anything, mock.Anything, "owner-uuid").
			Return(task.Task{}, unreachable).Once()

		_, err := NewUseCase(&runner, accepts(), directory(), ingressDomain).Execute(
			context.Background(),
			composeRequest(t, `{"name": "nginx", "image": "nginx:alpine"}`),
		)

		assert.ErrorIs(t, err, unreachable)
	})

	t.Run("a request the rules refuse never reaches the runner", func(t *testing.T) {
		t.Parallel()

		var runner manager.MockClient

		refusal := domain.ValidationErrors{"image": "required_field"}

		v := &validator.MockValidator{}
		v.On("Validate", mock.Anything).Return(refusal)

		response, err := NewUseCase(&runner, v, directory(), ingressDomain).Execute(
			context.Background(),
			composeRequest(t, `{"name": "nginx"}`),
		)

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)

		runner.AssertNotCalled(t, "RunContainer", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		body string
		want domain.ValidationErrors
	}{
		{
			name: "a name and an image are enough",
			body: `{"name": "nginx", "image": "nginx:alpine"}`,
			want: domain.ValidationErrors{},
		},
		{
			name: "a name is required, because the address is built from it",
			body: `{"image": "nginx:alpine"}`,
			want: domain.ValidationErrors{"name": "required_field"},
		},
		{
			name: "the service's own rules apply too",
			body: `{"name": "nginx", "network_mode": "none", "ports": ["80"]}`,
			want: domain.ValidationErrors{
				"image": "required_field",
				"ports": "ports_require_network",
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var request Request
			require.NoError(t, json.Unmarshal([]byte(tt.body), &request))
			request.OwnerUUID = "owner-uuid"

			assert.Equal(t, tt.want, request.Validate())
		})
	}
}
