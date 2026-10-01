package runTask

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/workload/spec"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	usersMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

const ingressDomain = "workload.localhost:8021"

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

// directory answers about whoever a task belongs to, which for these is
// nobody in particular: what is presented is covered where presenting is.
func directory() *presenter.Directory {
	users := &usersMock.MockUsersRepository{}
	users.On("GetByUUIDs", mock.Anything, mock.Anything).Return([]user.User{}, nil).Maybe()

	return presenter.NewDirectory(users)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("hands the specification to the workload as it was written", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		var handed workloadControlPlane.TaskSpec
		workload.On("RunTask", mock.Anything, mock.Anything, "owner-uuid").
			Run(func(args mock.Arguments) { handed = args.Get(1).(workloadControlPlane.TaskSpec) }).
			Return(task.Task{
				UUID:         "task-uuid",
				Name:         "nginx",
				Slug:         "nginx-xkfqz",
				CurrentState: task.Created,
				Endpoints:    []task.Endpoint{{TaskPort: 80}},
			}, nil).Once()
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, accepts(), directory(), ingressDomain).Execute(
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

		// answered with the addresses the task will be served on, so the
		// dashboard can link straight to it.
		require.NotNil(t, response.Task)
		assert.Equal(t, "nginx-xkfqz", response.Task.Slug)
		require.Len(t, response.Task.Endpoints, 1)
		assert.Equal(t, "http://nginx-xkfqz.workload.localhost:8021", response.Task.Endpoints[0].URL)
	})

	t.Run("what the workload refuses is reported as it stands", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		// the workload decides what it can run, so its verdict reaches the person
		// who asked rather than being flattened into a failure.
		refusal := domain.ValidationErrors{"exposed_ports": "a task with no network cannot expose ports"}

		workload.On("RunTask", mock.Anything, mock.Anything, "owner-uuid").
			Return(task.Task{}, &client.ValidationError{ValidationErrors: refusal}).Once()

		response, err := NewUseCase(&workload, accepts(), directory(), ingressDomain).Execute(
			context.Background(),
			composeRequest(t, `{"name": "nginx", "image": "nginx:alpine"}`),
		)

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)
		assert.Nil(t, response.Task)
	})

	t.Run("a workload that cannot be reached is a failure, not a refusal", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		unreachable := errors.New("the workload is unreachable")
		workload.On("RunTask", mock.Anything, mock.Anything, "owner-uuid").
			Return(task.Task{}, unreachable).Once()

		_, err := NewUseCase(&workload, accepts(), directory(), ingressDomain).Execute(
			context.Background(),
			composeRequest(t, `{"name": "nginx", "image": "nginx:alpine"}`),
		)

		assert.ErrorIs(t, err, unreachable)
	})

	t.Run("a request the rules refuse never reaches the workload", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		refusal := domain.ValidationErrors{"image": "required_field"}

		v := &validator.MockValidator{}
		v.On("Validate", mock.Anything).Return(refusal)

		response, err := NewUseCase(&workload, v, directory(), ingressDomain).Execute(
			context.Background(),
			composeRequest(t, `{"name": "nginx"}`),
		)

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)

		workload.AssertNotCalled(t, "RunTask", mock.Anything, mock.Anything, mock.Anything)
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
