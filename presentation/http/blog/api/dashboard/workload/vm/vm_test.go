package vm

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/deleteVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMLogs"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMs"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restartVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restoreVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/startVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/stopVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/updateVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/workloadhttptest"
)

// routes serves the VM routes of both sets, as the blog does.
func routes(client *controlplane.MockClient) *http.ServeMux {
	var (
		v       = workloadtest.Validator()
		t       = workloadtest.Translator()
		owners  = workloadtest.Owners()
		ingress = workloadtest.IngressDomain
		mux     = http.NewServeMux()
	)

	mux.Handle("POST /api/dashboard/workload/vms", NewCreateHandler(createVM.NewUseCase(client, v, t, owners, ingress)))

	for _, set := range workloadhttptest.Sets {
		mux.Handle("GET "+set.Prefix+"/vms", NewIndexHandler(getVMs.NewUseCase(client, v, owners, ingress), set.Owner))
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}", NewShowHandler(getVM.NewUseCase(client, owners, ingress), set.Owner))
		mux.Handle("PATCH "+set.Prefix+"/vms/{uuid}", NewUpdateHandler(updateVM.NewUseCase(client, v, t, owners, ingress), set.Owner))
		mux.Handle("DELETE "+set.Prefix+"/vms/{uuid}", NewDeleteHandler(deleteVM.NewUseCase(client, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/start", NewStartHandler(startVM.NewUseCase(client, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/stop", NewStopHandler(stopVM.NewUseCase(client, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/restart", NewRestartHandler(restartVM.NewUseCase(client, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/restore", NewRestoreHandler(restoreVM.NewUseCase(client, v, t), set.Owner))
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/logs", NewLogsHandler(getVMLogs.NewUseCase(client, t), set.Owner))
	}

	return mux
}

func TestTheRoutesOfBothSets(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 10, 4, 12, 1, 0, 0, time.UTC)

	for _, set := range workloadhttptest.Sets {
		testcases := []struct {
			name   string
			method string
			target string
			body   string
			expect func(client *controlplane.MockClient)
			status int
			answer string
		}{
			{
				name:   "a page of VMs, of one kind",
				method: http.MethodGet,
				target: "/vms?page=2&kind=docker",
				expect: func(client *controlplane.MockClient) {
					client.On("VMs", mock.Anything, set.OwnerUUID, vm.KindDocker, uint(2)).Once().Return(workloadControlPlane.Page[vm.VM]{
						Items: []vm.VM{workloadtest.VM()}, TotalPages: 2, CurrentPage: 2,
					}, nil)
				},
				status: http.StatusOK,
				answer: `{"items": [` + workloadtest.VMJSON + `], "pagination": {"total_pages": 2, "current_page": 2}}`,
			},
			{
				name:   "a kind that is not one",
				method: http.MethodGet,
				target: "/vms?kind=lxc",
				status: http.StatusBadRequest,
				answer: `{"errors": {"kind": "the kind must be either machine or docker"}}`,
			},
			{
				name:   "one VM",
				method: http.MethodGet,
				target: "/vms/vm-uuid",
				expect: func(client *controlplane.MockClient) {
					client.On("VM", mock.Anything, set.OwnerUUID, "vm-uuid").Once().Return(workloadtest.VM(), nil)
				},
				status: http.StatusOK,
				answer: workloadtest.VMJSON,
			},
			{
				name:   "one that is not there, or not the caller's",
				method: http.MethodGet,
				target: "/vms/gone-uuid",
				expect: func(client *controlplane.MockClient) {
					client.On("VM", mock.Anything, set.OwnerUUID, "gone-uuid").Once().Return(vm.VM{}, domain.ErrNotExists)
				},
				status: http.StatusNotFound,
				answer: `{"code": "not_found"}`,
			},
			{
				name:   "a change, answered with the VM as it is now",
				method: http.MethodPatch,
				target: "/vms/vm-uuid",
				body:   `{"name": "renamed"}`,
				expect: func(client *controlplane.MockClient) {
					client.On("UpdateVM", mock.Anything, set.OwnerUUID, "vm-uuid", mock.Anything).Once().Return(workloadtest.VM(), nil)
				},
				status: http.StatusOK,
				answer: workloadtest.VMJSON,
			},
			{
				name:   "a change the rules refuse, in the reader's language",
				method: http.MethodPatch,
				target: "/vms/vm-uuid",
				body:   `{"ports": [80, 80]}`,
				status: http.StatusBadRequest,
				answer: `{"errors": {"ports.1": "the same port is listed more than once"}}`,
			},
			{
				name:   "a change that is not json",
				method: http.MethodPatch,
				target: "/vms/vm-uuid",
				body:   `{"name":`,
				status: http.StatusBadRequest,
				answer: `{"errors": {"body": "invalid_value"}}`,
			},
			{
				name:   "a delete, done in the workload's own time",
				method: http.MethodDelete,
				target: "/vms/vm-uuid",
				expect: func(client *controlplane.MockClient) {
					client.On("DeleteVM", mock.Anything, set.OwnerUUID, "vm-uuid").Once().Return(nil)
				},
				status: http.StatusAccepted,
			},
			{
				name:   "a start",
				method: http.MethodPost,
				target: "/vms/vm-uuid/start",
				expect: func(client *controlplane.MockClient) {
					client.On("StartVM", mock.Anything, set.OwnerUUID, "vm-uuid").Once().Return(nil)
				},
				status: http.StatusAccepted,
			},
			{
				name:   "a stop",
				method: http.MethodPost,
				target: "/vms/vm-uuid/stop",
				expect: func(client *controlplane.MockClient) {
					client.On("StopVM", mock.Anything, set.OwnerUUID, "vm-uuid").Once().Return(nil)
				},
				status: http.StatusAccepted,
			},
			{
				name:   "a restart the control plane refuses",
				method: http.MethodPost,
				target: "/vms/vm-uuid/restart",
				expect: func(client *controlplane.MockClient) {
					client.On("RestartVM", mock.Anything, set.OwnerUUID, "vm-uuid").Once().Return(&noderequest.Error{Code: noderequest.CodeNotRunning})
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"vm": "the VM is not running"}}`,
			},
			{
				name:   "a restore from a snapshot",
				method: http.MethodPost,
				target: "/vms/vm-uuid/restore",
				body:   `{"snapshot_uuid": "snapshot-uuid"}`,
				expect: func(client *controlplane.MockClient) {
					client.On("RestoreVM", mock.Anything, set.OwnerUUID, "vm-uuid", "snapshot-uuid").Once().Return(nil)
				},
				status: http.StatusAccepted,
			},
			{
				name:   "the tail of the log since a moment",
				method: http.MethodGet,
				target: "/vms/vm-uuid/logs?since=2026-10-04T12:01:00Z&tail=500",
				expect: func(client *controlplane.MockClient) {
					client.On("VMLogs", mock.Anything, set.OwnerUUID, "vm-uuid", vm.LogOptions{Since: since, Tail: 500}).Once().Return([]vm.LogLine{
						{At: since, Source: "main", Line: "ready"},
					}, nil)
				},
				status: http.StatusOK,
				answer: `{"items": [{"at": "2026-10-04T12:01:00Z", "source": "main", "line": "ready"}], "truncated": false}`,
			},
			{
				name:   "a log that took too long to read",
				method: http.MethodGet,
				target: "/vms/vm-uuid/logs",
				expect: func(client *controlplane.MockClient) {
					client.On("VMLogs", mock.Anything, set.OwnerUUID, "vm-uuid", vm.LogOptions{}).Once().Return(nil, &noderequest.Error{Code: noderequest.CodeTimeout})
				},
				status: http.StatusGatewayTimeout,
				answer: `{"code": "timeout"}`,
			},
		}

		for _, tt := range testcases {
			t.Run(set.Name+": "+tt.name, func(t *testing.T) {
				t.Parallel()

				var client controlplane.MockClient
				if tt.expect != nil {
					tt.expect(&client)
				}
				defer client.AssertExpectations(t)

				response := workloadhttptest.Serve(routes(&client), tt.method, set.Prefix+tt.target, tt.body)

				assert.Equal(t, tt.status, response.Code)
				if len(tt.answer) == 0 {
					assert.Empty(t, response.Body.String())

					return
				}

				assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
				assert.JSONEq(t, tt.answer, response.Body.String())
			})
		}
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()

	t.Run("a VM is created for whoever asks, and answered with as itself", func(t *testing.T) {
		t.Parallel()

		var client controlplane.MockClient
		client.On("CreateVM", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(workloadtest.VM(), nil)
		defer client.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&client), http.MethodPost, "/api/dashboard/workload/vms", `{
			"name": "docker-1",
			"kind": "docker",
			"resources": {"cpus": 2, "memory": 2147483648, "disk": 21474836480},
			"ports": [80, 8080],
			"network": {"ingress": "allow", "egress": "allow"},
			"persistent_disk": true,
			"lifetime_seconds": 0
		}`)

		assert.Equal(t, http.StatusCreated, response.Code)
		assert.JSONEq(t, workloadtest.VMJSON, response.Body.String())
	})

	t.Run("what the control plane refuses is said by field", func(t *testing.T) {
		t.Parallel()

		var client controlplane.MockClient
		client.On("CreateVM", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(vm.VM{}, vm.ErrQuotaExceeded)
		defer client.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&client), http.MethodPost, "/api/dashboard/workload/vms", `{
			"name": "one too many",
			"kind": "machine",
			"resources": {"cpus": 1, "memory": 1073741824, "disk": 10737418240},
			"network": {"ingress": "deny", "egress": "deny"}
		}`)

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(t, `{"errors": {"vm": "this would take you past your quota"}}`, response.Body.String())
	})

	t.Run("a workload that cannot be reached is a failure, and says nothing more", func(t *testing.T) {
		t.Parallel()

		var client controlplane.MockClient
		client.On("CreateVM", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(vm.VM{}, workloadtest.ErrUnreachable)
		defer client.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&client), http.MethodPost, "/api/dashboard/workload/vms", `{
			"name": "web",
			"kind": "machine",
			"resources": {"cpus": 1, "memory": 1073741824, "disk": 10737418240},
			"network": {"ingress": "deny", "egress": "deny"}
		}`)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.JSONEq(t, `{"code": "internal"}`, response.Body.String())
	})
}
