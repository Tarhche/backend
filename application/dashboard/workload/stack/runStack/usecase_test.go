package runStack

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/workload/spec"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	usersMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

const ingressDomain = "workload.localhost:8030"

// codes is a validator that refuses what a request says is wrong with itself,
// as the codes it says it with, so a test can read what was refused.
type codes struct{}

var _ domain.Validator = codes{}

func (codes) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

func directory() *presenter.Directory {
	users := &usersMock.MockUsersRepository{}
	users.On("GetByUUIDs", mock.Anything, mock.Anything).Return([]user.User{}, nil).Maybe()

	return presenter.NewDirectory(users)
}

func composeRequest(t *testing.T, body string) *Request {
	t.Helper()

	var request Request
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	request.OwnerUUID = "owner-uuid"

	return &request
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ran := workloadControlPlane.Stack{
		Stack: stack.Stack{UUID: "stack-uuid", Name: "myapp", Runtime: runtime.Firecracker, OwnerUUID: "owner-uuid", CreatedAt: time.Now()},
		Services: []task.Task{
			{UUID: "web-uuid", ServiceName: "web", Runtime: runtime.Firecracker, NodeName: "workload-orchestrator-01"},
		},
	}

	t.Run("a class named for the stack is every service's that names none", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		var handed workloadControlPlane.StackSpec
		workload.On("RunStack", mock.Anything, mock.Anything, "owner-uuid").
			Run(func(args mock.Arguments) { handed = args.Get(1).(workloadControlPlane.StackSpec) }).
			Return(ran, nil).Once()
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, codes{}, directory(), ingressDomain).Execute(context.Background(), composeRequest(t, `{
			"name": "myapp",
			"runtime": "firecracker",
			"services": {"web": {"image": "nginx:alpine"}, "db": {"image": "postgres:17", "runtime": "firecracker"}}
		}`))
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		services, ok := handed.Services.(map[string]spec.Service)
		require.True(t, ok, "the services travel in the shape they were written")
		assert.Equal(t, runtime.Firecracker, services["web"].Runtime)
		assert.Equal(t, runtime.Firecracker, services["db"].Runtime)

		require.NotNil(t, response.Stack)
		assert.Equal(t, "firecracker", response.Stack.Runtime)
		require.Len(t, response.Stack.Services, 1)
		assert.Equal(t, "firecracker", response.Stack.Services[0].Runtime)
		assert.Equal(t, "workload-orchestrator-01", response.Stack.Services[0].Node)
	})

	t.Run("a stack naming no class hands its services on as they were written", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		var handed workloadControlPlane.StackSpec
		workload.On("RunStack", mock.Anything, mock.Anything, "owner-uuid").
			Run(func(args mock.Arguments) { handed = args.Get(1).(workloadControlPlane.StackSpec) }).
			Return(ran, nil).Once()

		_, err := NewUseCase(&workload, codes{}, directory(), ingressDomain).Execute(context.Background(), composeRequest(t, `{
			"name": "myapp",
			"services": {"web": {"image": "nginx:alpine"}}
		}`))
		require.NoError(t, err)

		services := handed.Services.(map[string]spec.Service)

		// left to the workload's default, which only the workload knows.
		assert.Empty(t, services["web"].Runtime)
	})

	t.Run("services naming two classes are refused before the workload is asked anything", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := NewUseCase(&workload, codes{}, directory(), ingressDomain).Execute(context.Background(), composeRequest(t, `{
			"name": "myapp",
			"runtime": "firecracker",
			"services": {"web": {"image": "nginx:alpine"}, "db": {"image": "postgres:17", "runtime": "sysbox"}}
		}`))
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"runtime": "mixed_runtimes_in_stack"}, response.ValidationErrors)
		workload.AssertNotCalled(t, "RunStack", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a class the workload does not allow is refused by it, as it stands", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		refusal := domain.ValidationErrors{"runtime": "the provided value is invalid"}
		workload.On("RunStack", mock.Anything, mock.Anything, "owner-uuid").
			Return(workloadControlPlane.Stack{}, &client.ValidationError{ValidationErrors: refusal}).Once()

		response, err := NewUseCase(&workload, codes{}, directory(), ingressDomain).Execute(context.Background(), composeRequest(t, `{
			"name": "myapp",
			"runtime": "gvisor",
			"services": {"web": {"image": "nginx:alpine"}}
		}`))
		require.NoError(t, err)

		assert.Equal(t, refusal, response.ValidationErrors)
	})
}
