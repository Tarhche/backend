package stack

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/createStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/deleteStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStacks"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/restartStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/startStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/stopStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/workloadhttptest"
)

func routes(workload *controlplane.MockClient) *http.ServeMux {
	var (
		v      = workloadtest.Validator()
		t      = workloadtest.Translator()
		owners = workloadtest.Owners()
		mux    = http.NewServeMux()
	)

	mux.Handle("POST /api/dashboard/workload/stacks", NewCreateHandler(createStack.NewUseCase(workload, v, t, owners)))

	for _, set := range workloadhttptest.Sets {
		mux.Handle("GET "+set.Prefix+"/stacks", NewIndexHandler(getStacks.NewUseCase(workload, owners), set.Owner))
		mux.Handle("GET "+set.Prefix+"/stacks/{uuid}", NewShowHandler(getStack.NewUseCase(workload, owners), set.Owner))
		mux.Handle("DELETE "+set.Prefix+"/stacks/{uuid}", NewDeleteHandler(deleteStack.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/stacks/{uuid}/start", NewStartHandler(startStack.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/stacks/{uuid}/stop", NewStopHandler(stopStack.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/stacks/{uuid}/restart", NewRestartHandler(restartStack.NewUseCase(workload, t), set.Owner))
	}

	return mux
}

func TestTheRoutesOfBothSets(t *testing.T) {
	t.Parallel()

	for _, set := range workloadhttptest.Sets {
		testcases := []struct {
			name   string
			method string
			target string
			expect func(workload *controlplane.MockClient)
			status int
			answer string
		}{
			{
				name:   "a page of one VM's stacks",
				method: http.MethodGet,
				target: "/stacks?vm=vm-uuid",
				expect: func(workload *controlplane.MockClient) {
					workload.On("Stacks", mock.Anything, set.OwnerUUID, "vm-uuid", uint(1)).Once().Return(workloadControlPlane.Page[stack.Stack]{CurrentPage: 1}, nil)
				},
				status: http.StatusOK,
				answer: `{"items": [], "pagination": {"total_pages": 0, "current_page": 1}}`,
			},
			{
				name:   "one stack whose VM is not running",
				method: http.MethodGet,
				target: "/stacks/stack-uuid",
				expect: func(workload *controlplane.MockClient) {
					workload.On("Stack", mock.Anything, set.OwnerUUID, "stack-uuid").Once().Return(workloadControlPlane.StackDetail{
						Stack:        workloadtest.Stack(),
						VMNotRunning: true,
					}, nil)
				},
				status: http.StatusOK,
				answer: workloadtest.StackJSON[:len(workloadtest.StackJSON)-1] + `, "containers": [], "note": "vm_not_running"}`,
			},
			{
				name:   "one that is not there, or not the caller's",
				method: http.MethodGet,
				target: "/stacks/gone-uuid",
				expect: func(workload *controlplane.MockClient) {
					workload.On("Stack", mock.Anything, set.OwnerUUID, "gone-uuid").Once().Return(workloadControlPlane.StackDetail{}, domain.ErrNotExists)
				},
				status: http.StatusNotFound,
				answer: `{"code": "not_found"}`,
			},
			{
				name:   "a delete that keeps the volumes",
				method: http.MethodDelete,
				target: "/stacks/stack-uuid",
				expect: func(workload *controlplane.MockClient) {
					workload.On("DeleteStack", mock.Anything, set.OwnerUUID, "stack-uuid", false).Once().Return(nil)
				},
				status: http.StatusAccepted,
			},
			{
				name:   "a delete that takes them too",
				method: http.MethodDelete,
				target: "/stacks/stack-uuid?volumes=true",
				expect: func(workload *controlplane.MockClient) {
					workload.On("DeleteStack", mock.Anything, set.OwnerUUID, "stack-uuid", true).Once().Return(nil)
				},
				status: http.StatusAccepted,
			},
			{
				name:   "a start",
				method: http.MethodPost,
				target: "/stacks/stack-uuid/start",
				expect: func(workload *controlplane.MockClient) {
					workload.On("StartStack", mock.Anything, set.OwnerUUID, "stack-uuid").Once().Return(nil)
				},
				status: http.StatusAccepted,
			},
			{
				name:   "a stop",
				method: http.MethodPost,
				target: "/stacks/stack-uuid/stop",
				expect: func(workload *controlplane.MockClient) {
					workload.On("StopStack", mock.Anything, set.OwnerUUID, "stack-uuid").Once().Return(nil)
				},
				status: http.StatusAccepted,
			},
			{
				name:   "a restart the control plane refuses",
				method: http.MethodPost,
				target: "/stacks/stack-uuid/restart",
				expect: func(workload *controlplane.MockClient) {
					workload.On("RestartStack", mock.Anything, set.OwnerUUID, "stack-uuid").Once().Return(
						&client.ValidationError{ValidationErrors: domain.ValidationErrors{"state": "invalid_state_transition"}},
					)
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"state": "invalid state transition"}}`,
			},
		}

		for _, tt := range testcases {
			t.Run(set.Name+": "+tt.name, func(t *testing.T) {
				t.Parallel()

				var workload controlplane.MockClient
				if tt.expect != nil {
					tt.expect(&workload)
				}
				defer workload.AssertExpectations(t)

				response := workloadhttptest.Serve(routes(&workload), tt.method, set.Prefix+tt.target, "")

				assert.Equal(t, tt.status, response.Code)
				if len(tt.answer) == 0 {
					assert.Empty(t, response.Body.String())

					return
				}

				assert.JSONEq(t, tt.answer, response.Body.String())
			})
		}
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()

	t.Run("a stack is deployed for whoever asks, and says where it went", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateStack", mock.Anything, workloadtest.OwnerUUID, workloadControlPlane.StackRequest{
			Name:    "shop",
			Compose: "services:\n  web:\n    image: nginx:1.27\n",
			VM:      workloadControlPlane.DockerVMChoice{UUID: "vm-uuid"},
		}).Once().Return(workloadControlPlane.CreatedStack{
			VM:    workloadControlPlane.ChosenVM{UUID: "vm-uuid", Name: "docker-1"},
			Stack: workloadtest.Stack(),
		}, nil)
		defer workload.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&workload), http.MethodPost, "/api/dashboard/workload/stacks", `{
			"name": "shop",
			"compose": "services:\n  web:\n    image: nginx:1.27\n",
			"vm_uuid": "vm-uuid"
		}`)

		assert.Equal(t, http.StatusCreated, response.Code)
		assert.JSONEq(t, `{"vm": {"uuid": "vm-uuid", "name": "docker-1", "created": false}, "stack": `+workloadtest.StackJSON+`}`, response.Body.String())
	})

	t.Run("a Docker VM no node has room for, made for it as it named none", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateStack", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(
			workloadControlPlane.CreatedStack{},
			&client.ValidationError{ValidationErrors: domain.ValidationErrors{"vm": "no_capacity"}},
		)
		defer workload.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&workload), http.MethodPost, "/api/dashboard/workload/stacks", `{"name": "shop", "compose": "services: {}"}`)

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(t, `{"errors": {"vm": "there is no room for it on any node right now, try again later"}}`, response.Body.String())
	})
}
