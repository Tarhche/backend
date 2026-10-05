package container

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/connectNetwork"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/createContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/deleteContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/disconnectNetwork"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainerLogs"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainerStats"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainers"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getVMContainers"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/restartContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/startContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/stopContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	dockerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/workloadhttptest"
)

func routes(workload *controlplane.MockClient) *http.ServeMux {
	var (
		v   = workloadtest.Validator()
		t   = workloadtest.Translator()
		mux = http.NewServeMux()
	)

	mux.Handle("POST /api/dashboard/workload/containers", NewCreateHandler(createContainer.NewUseCase(workload, v, t)))

	for _, set := range workloadhttptest.Sets {
		mux.Handle("GET "+set.Prefix+"/containers", NewIndexHandler(getContainers.NewUseCase(workload, t), set.Owner))
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/containers", NewVMIndexHandler(getVMContainers.NewUseCase(workload, t), set.Owner))
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/containers/{id}", NewShowHandler(getContainer.NewUseCase(workload, t), set.Owner))
		mux.Handle("DELETE "+set.Prefix+"/vms/{uuid}/containers/{id}", NewDeleteHandler(deleteContainer.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/containers/{id}/start", NewStartHandler(startContainer.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/containers/{id}/stop", NewStopHandler(stopContainer.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/containers/{id}/restart", NewRestartHandler(restartContainer.NewUseCase(workload, t), set.Owner))
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/containers/{id}/logs", NewLogsHandler(getContainerLogs.NewUseCase(workload, t), set.Owner))
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/containers/{id}/stats", NewStatsHandler(getContainerStats.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/containers/{id}/networks", NewConnectHandler(connectNetwork.NewUseCase(workload, v, t), set.Owner))
		mux.Handle("DELETE "+set.Prefix+"/vms/{uuid}/containers/{id}/networks/{network}", NewDisconnectHandler(disconnectNetwork.NewUseCase(workload, t), set.Owner))
	}

	return mux
}

// noStacks is a VM with no stacks in it, which is what a container listing
// asks about besides its containers.
func noStacks(workload *controlplane.MockClient, ownerUUID string, vmUUID string) {
	workload.On("Stacks", mock.Anything, ownerUUID, vmUUID, uint(1)).Once().Return(workloadControlPlane.Page[stack.Stack]{CurrentPage: 1}, nil)
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
			expect func(workload *controlplane.MockClient, daemon *dockerMock.MockDaemon)
			status int
			answer string
		}{
			{
				name:   "the containers across the Docker VMs",
				method: http.MethodGet,
				target: "/containers?vm=vm-uuid",
				expect: func(workload *controlplane.MockClient, _ *dockerMock.MockDaemon) {
					workload.On("Containers", mock.Anything, set.OwnerUUID, "vm-uuid").Once().Return([]workloadControlPlane.VMContainer{
						{Container: docker.Container{ID: "c0ffee", Name: "web", State: "running", CreatedAt: workloadtest.At}, VMUUID: "vm-uuid", VMName: "docker-1"},
					}, nil)
					noStacks(workload, set.OwnerUUID, "vm-uuid")
				},
				status: http.StatusOK,
				answer: `{"items": [{
					"id": "c0ffee", "name": "web", "image": "", "state": "running", "status": "", "command": "",
					"ports": [], "networks": [], "mounts": [], "created_at": "2026-10-04T12:00:00Z",
					"vm_uuid": "vm-uuid", "vm_name": "docker-1"
				}]}`,
			},
			{
				name:   "one Docker VM's containers",
				method: http.MethodGet,
				target: "/vms/vm-uuid/containers",
				expect: func(workload *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("Containers", mock.Anything, docker.ContainerFilter{All: true}).Once().Return(nil, nil)
					noStacks(workload, set.OwnerUUID, "vm-uuid")
				},
				status: http.StatusOK,
				answer: `{"items": []}`,
			},
			{
				name:   "the containers of a VM that is not running",
				method: http.MethodGet,
				target: "/vms/vm-uuid/containers",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("Containers", mock.Anything, docker.ContainerFilter{All: true}).Once().Return(nil, &noderequest.Error{Code: noderequest.CodeNotRunning})
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"vm": "the VM is not running"}}`,
			},
			{
				name:   "one container, by name",
				method: http.MethodGet,
				target: "/vms/vm-uuid/containers/web",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("Container", mock.Anything, "web").Once().Return(docker.Container{ID: "c0ffee", Name: "web", CreatedAt: workloadtest.At}, nil)
				},
				status: http.StatusOK,
				answer: `{"id": "c0ffee", "name": "web", "image": "", "state": "", "status": "", "command": "", "ports": [], "networks": [], "mounts": [], "created_at": "2026-10-04T12:00:00Z"}`,
			},
			{
				name:   "one that is not there",
				method: http.MethodGet,
				target: "/vms/vm-uuid/containers/nope",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("Container", mock.Anything, "nope").Once().Return(docker.Container{}, &noderequest.Error{Code: noderequest.CodeNotFound, Message: "No such container: nope"})
				},
				status: http.StatusNotFound,
				answer: `{"code": "not_found", "message": "No such container: nope"}`,
			},
			{
				name:   "a removal, forced",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/containers/c0ffee?force=true",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("RemoveContainer", mock.Anything, "c0ffee", true).Once().Return(nil)
				},
				status: http.StatusNoContent,
			},
			{
				name:   "a removal, not",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/containers/c0ffee",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("RemoveContainer", mock.Anything, "c0ffee", false).Once().Return(&noderequest.Error{
						Code:    noderequest.CodeInvalid,
						Message: "cannot remove container: container is running",
					})
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"docker": "Docker refused the request: cannot remove container: container is running"}}`,
			},
			{
				name:   "a start",
				method: http.MethodPost,
				target: "/vms/vm-uuid/containers/c0ffee/start",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("StartContainer", mock.Anything, "c0ffee").Once().Return(nil)
				},
				status: http.StatusNoContent,
			},
			{
				name:   "a stop",
				method: http.MethodPost,
				target: "/vms/vm-uuid/containers/c0ffee/stop",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("StopContainer", mock.Anything, "c0ffee").Once().Return(nil)
				},
				status: http.StatusNoContent,
			},
			{
				name:   "a restart",
				method: http.MethodPost,
				target: "/vms/vm-uuid/containers/c0ffee/restart",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("RestartContainer", mock.Anything, "c0ffee").Once().Return(nil)
				},
				status: http.StatusNoContent,
			},
			{
				name:   "the tail of its log since a moment",
				method: http.MethodGet,
				target: "/vms/vm-uuid/containers/c0ffee/logs?since=2026-10-04T12:01:00Z&tail=100",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("ContainerLogs", mock.Anything, "c0ffee", docker.LogOptions{Since: since, Tail: 100}).Once().Return([]docker.LogLine{
						{At: since, Stream: "stdout", Line: "ready"},
					}, nil)
				},
				status: http.StatusOK,
				answer: `{"items": [{"at": "2026-10-04T12:01:00Z", "stream": "stdout", "line": "ready"}], "truncated": false}`,
			},
			{
				name:   "a sample of what it uses",
				method: http.MethodGet,
				target: "/vms/vm-uuid/containers/c0ffee/stats",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("ContainerStats", mock.Anything, "c0ffee").Once().Return(docker.Stats{CPUPercent: 1, SampledAt: workloadtest.At}, nil)
				},
				status: http.StatusOK,
				answer: `{"cpu_percent": 1, "memory_used": 0, "memory_limit": 0, "network_rx": 0, "network_tx": 0, "block_read": 0, "block_write": 0, "pids": 0, "sampled_at": "2026-10-04T12:00:00Z"}`,
			},
			{
				name:   "connected to a network, under an alias",
				method: http.MethodPost,
				target: "/vms/vm-uuid/containers/c0ffee/networks",
				body:   `{"network": "backend", "aliases": ["api"]}`,
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("ConnectNetwork", mock.Anything, "backend", "c0ffee", []string{"api"}).Once().Return(nil)
				},
				status: http.StatusNoContent,
			},
			{
				name:   "connected to no network at all",
				method: http.MethodPost,
				target: "/vms/vm-uuid/containers/c0ffee/networks",
				body:   `{}`,
				status: http.StatusBadRequest,
				answer: `{"errors": {"network": "this field is required"}}`,
			},
			{
				name:   "disconnected from a network",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/containers/c0ffee/networks/backend",
				expect: func(_ *controlplane.MockClient, daemon *dockerMock.MockDaemon) {
					daemon.On("DisconnectNetwork", mock.Anything, "backend", "c0ffee", false).Once().Return(nil)
				},
				status: http.StatusNoContent,
			},
		}

		for _, tt := range testcases {
			t.Run(set.Name+": "+tt.name, func(t *testing.T) {
				t.Parallel()

				var (
					workload controlplane.MockClient
					daemon   dockerMock.MockDaemon
				)

				// a Docker VM is asked for as the set's owner, so on the my
				// routes one that is not the caller's is not there.
				workload.On("Docker", set.OwnerUUID, "vm-uuid").Maybe().Return(&daemon)

				if tt.expect != nil {
					tt.expect(&workload, &daemon)
				}
				defer workload.AssertExpectations(t)
				defer daemon.AssertExpectations(t)

				response := workloadhttptest.Serve(routes(&workload), tt.method, set.Prefix+tt.target, tt.body)

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

	t.Run("a container is created for whoever asks, and says where it went", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateContainer", mock.Anything, workloadtest.OwnerUUID, workloadControlPlane.ContainerRequest{
			Container: docker.ContainerSpec{Image: "nginx:1.27"},
		}).Once().Return(workloadControlPlane.CreatedContainer{
			VM:        workloadControlPlane.ChosenVM{UUID: "vm-uuid", Name: "docker-1", Created: true},
			Container: docker.Container{ID: "c0ffee", Image: "nginx:1.27", State: "running", CreatedAt: workloadtest.At},
		}, nil)
		defer workload.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&workload), http.MethodPost, "/api/dashboard/workload/containers", `{"image": "nginx:1.27"}`)

		assert.Equal(t, http.StatusCreated, response.Code)
		assert.JSONEq(t, `{
			"vm": {"uuid": "vm-uuid", "name": "docker-1", "created": true},
			"container": {"id": "c0ffee", "name": "", "image": "nginx:1.27", "state": "running", "status": "", "command": "", "ports": [], "networks": [], "mounts": [], "created_at": "2026-10-04T12:00:00Z"}
		}`, response.Body.String())
	})

	t.Run("a pull that took longer than the control plane waits may still be under way", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateContainer", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(
			workloadControlPlane.CreatedContainer{},
			&noderequest.Error{Code: noderequest.CodeTimeout, Message: "no answer in 10m0s"},
		)
		defer workload.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&workload), http.MethodPost, "/api/dashboard/workload/containers", `{"image": "huge:latest"}`)

		assert.Equal(t, http.StatusGatewayTimeout, response.Code)
		assert.JSONEq(t, `{"code": "timeout", "message": "no answer in 10m0s"}`, response.Body.String())
	})
}
