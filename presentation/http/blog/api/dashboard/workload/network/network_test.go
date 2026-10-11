package network

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/createNetwork"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/deleteNetwork"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/getNetworkDependents"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/getNetworks"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
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

	for _, set := range workloadhttptest.Sets {
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/networks", NewIndexHandler(getNetworks.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/networks", NewCreateHandler(createNetwork.NewUseCase(workload, v, t), set.Owner))
		mux.Handle("DELETE "+set.Prefix+"/vms/{uuid}/networks/{id}", NewDeleteHandler(deleteNetwork.NewUseCase(workload, t), set.Owner))
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/networks/{id}/dependents", NewDependentsHandler(getNetworkDependents.NewUseCase(workload, t), set.Owner))
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
			body   string
			expect func(daemon *dockerMock.MockDaemon)
			asked  func(workload *controlplane.MockClient, ownerUUID string)
			status int
			answer string
		}{
			{
				name:   "what a Docker VM holds",
				method: http.MethodGet,
				target: "/vms/vm-uuid/networks",
				expect: func(daemon *dockerMock.MockDaemon) {
					daemon.On("Networks", mock.Anything).Once().Return([]docker.Network{{ID: "n1", Name: "bridge", Driver: "bridge", CreatedAt: workloadtest.At}}, nil)
				},
				status: http.StatusOK,
				answer: `{"items": [{"id": "n1", "name": "bridge", "driver": "bridge", "scope": "", "internal": false, "containers": [], "created_at": "2026-10-04T12:00:00Z"}]}`,
			},
			{
				name:   "what a Docker VM holds, when it is not running",
				method: http.MethodGet,
				target: "/vms/vm-uuid/networks",
				expect: func(daemon *dockerMock.MockDaemon) {
					daemon.On("Networks", mock.Anything).Once().Return(nil, &noderequest.Error{Code: noderequest.CodeDockerUnavailable})
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"vm": "Docker did not come up in the VM in time, try again shortly"}}`,
			},
			{
				name:   "one more, inside the VM",
				method: http.MethodPost,
				target: "/vms/vm-uuid/networks",
				body:   `{"name": "backend", "driver": "bridge", "internal": false}`,
				expect: func(daemon *dockerMock.MockDaemon) {
					daemon.On("CreateNetwork", mock.Anything, docker.NetworkSpec{Name: "backend", Driver: "bridge"}).Once().Return(docker.Network{ID: "n1", Name: "backend", Driver: "bridge", CreatedAt: workloadtest.At}, nil)
				},
				status: http.StatusCreated,
				answer: `{"id": "n1", "name": "backend", "driver": "bridge", "scope": "", "internal": false, "containers": [], "created_at": "2026-10-04T12:00:00Z"}`,
			},
			{
				name:   "one asked for wrongly is refused before the VM is asked",
				method: http.MethodPost,
				target: "/vms/vm-uuid/networks",
				body:   `{}`,
				status: http.StatusBadRequest,
			},
			{
				name:   "a removal",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/networks/backend",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("DeleteNetwork", mock.Anything, ownerUUID, "vm-uuid", "backend", false).Once().Return(workloadControlPlane.Deleted{}, nil)
				},
				status: http.StatusNoContent,
			},
			{
				name:   "a removal of what a container needs is answered with it, and whether it makes it again",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/networks/backend",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("DeleteNetwork", mock.Anything, ownerUUID, "vm-uuid", "backend", false).Once().Return(workloadControlPlane.Deleted{
						Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "web", Recreates: true}},
						Recreated:  true,
					}, nil)
				},
				status: http.StatusAccepted,
				answer: `{"dependents": [{"kind": "container", "uuid": "c-uuid", "name": "web", "recreates": true}], "recreated": true}`,
			},
			{
				name:   "and with cascade, it goes first",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/networks/backend?cascade=true",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("DeleteNetwork", mock.Anything, ownerUUID, "vm-uuid", "backend", true).Once().Return(workloadControlPlane.Deleted{
						Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "web", Recreates: true}},
					}, nil)
				},
				status: http.StatusAccepted,
				answer: `{"dependents": [{"kind": "container", "uuid": "c-uuid", "name": "web", "recreates": true}], "recreated": false}`,
			},
			{
				name:   "a removal refused is answered with what needs it beside the refusal",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/networks/backend",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("DeleteNetwork", mock.Anything, ownerUUID, "vm-uuid", "backend", false).Once().Return(workloadControlPlane.Deleted{
						Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "web", Recreates: true}},
						Recreated:  true,
					}, &noderequest.Error{Code: noderequest.CodeInvalid, Message: "network backend has active endpoints"})
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"docker": "Docker refused the request: network backend has active endpoints"}, "dependents": [{"kind": "container", "uuid": "c-uuid", "name": "web", "recreates": true}], "recreated": true}`,
			},
			{
				name:   "what needs it, read and nothing deleted",
				method: http.MethodGet,
				target: "/vms/vm-uuid/networks/backend/dependents",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("NetworkDependents", mock.Anything, ownerUUID, "vm-uuid", "backend").Once().Return(workloadControlPlane.Deleted{
						Dependents: []workloadControlPlane.Dependent{{Kind: "stack", UUID: "stack-uuid", Name: "shop", Recreates: true}},
						Recreated:  true,
					}, nil)
				},
				status: http.StatusOK,
				answer: `{"dependents": [{"kind": "stack", "uuid": "stack-uuid", "name": "shop", "recreates": true}], "recreated": true}`,
			},
			{
				name:   "and of one nothing needs, a list of none",
				method: http.MethodGet,
				target: "/vms/vm-uuid/networks/backend/dependents",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("NetworkDependents", mock.Anything, ownerUUID, "vm-uuid", "backend").Once().Return(workloadControlPlane.Deleted{}, nil)
				},
				status: http.StatusOK,
				answer: `{"dependents": [], "recreated": false}`,
			},
			{
				name:   "and of one that is not there, or not the caller's, nothing",
				method: http.MethodGet,
				target: "/vms/vm-uuid/networks/gone/dependents",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("NetworkDependents", mock.Anything, ownerUUID, "vm-uuid", "gone").Once().Return(workloadControlPlane.Deleted{}, domain.ErrNotExists)
				},
				status: http.StatusNotFound,
				answer: `{"code": "not_found"}`,
			},
			{
				name:   "and of one in a vm that is not running, refused as its removal is",
				method: http.MethodGet,
				target: "/vms/vm-uuid/networks/backend/dependents",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("NetworkDependents", mock.Anything, ownerUUID, "vm-uuid", "backend").Once().Return(workloadControlPlane.Deleted{}, &noderequest.Error{Code: noderequest.CodeNotRunning})
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"vm": "the VM is not running"}}`,
			},
		}

		for _, tt := range testcases {
			t.Run(set.Name+": "+tt.name, func(t *testing.T) {
				t.Parallel()

				var (
					workload controlplane.MockClient
					daemon   dockerMock.MockDaemon
				)

				workload.On("Docker", set.OwnerUUID, "vm-uuid").Maybe().Return(&daemon)

				if tt.expect != nil {
					tt.expect(&daemon)
				}

				if tt.asked != nil {
					tt.asked(&workload, set.OwnerUUID)
				}
				defer workload.AssertExpectations(t)
				defer daemon.AssertExpectations(t)

				response := workloadhttptest.Serve(routes(&workload), tt.method, set.Prefix+tt.target, tt.body)

				assert.Equal(t, tt.status, response.Code)

				switch {
				case len(tt.answer) > 0:
					assert.JSONEq(t, tt.answer, response.Body.String())
				case tt.status == http.StatusBadRequest:
					assert.Contains(t, response.Body.String(), `"errors"`)
				default:
					assert.Empty(t, response.Body.String())
				}
			})
		}
	}
}
