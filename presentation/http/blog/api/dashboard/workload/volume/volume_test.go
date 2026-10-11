package volume

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/createVolume"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/deleteVolume"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/getVolumeDependents"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/getVolumes"
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
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/volumes", NewIndexHandler(getVolumes.NewUseCase(workload, t), set.Owner))
		mux.Handle("POST "+set.Prefix+"/vms/{uuid}/volumes", NewCreateHandler(createVolume.NewUseCase(workload, v, t), set.Owner))
		mux.Handle("DELETE "+set.Prefix+"/vms/{uuid}/volumes/{name}", NewDeleteHandler(deleteVolume.NewUseCase(workload, t), set.Owner))
		mux.Handle("GET "+set.Prefix+"/vms/{uuid}/volumes/{name}/dependents", NewDependentsHandler(getVolumeDependents.NewUseCase(workload, t), set.Owner))
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
				target: "/vms/vm-uuid/volumes",
				expect: func(daemon *dockerMock.MockDaemon) {
					daemon.On("Volumes", mock.Anything).Once().Return([]docker.Volume{{Name: "data", CreatedAt: workloadtest.At}}, nil)
				},
				status: http.StatusOK,
				answer: `{"items": [{"name": "data", "driver": "", "mountpoint": "", "in_use": false, "created_at": "2026-10-04T12:00:00Z"}]}`,
			},
			{
				name:   "what a Docker VM holds, when it is not running",
				method: http.MethodGet,
				target: "/vms/vm-uuid/volumes",
				expect: func(daemon *dockerMock.MockDaemon) {
					daemon.On("Volumes", mock.Anything).Once().Return(nil, &noderequest.Error{Code: noderequest.CodeDockerUnavailable})
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"vm": "Docker did not come up in the VM in time, try again shortly"}}`,
			},
			{
				name:   "one more, inside the VM",
				method: http.MethodPost,
				target: "/vms/vm-uuid/volumes",
				body:   `{"name": "data"}`,
				expect: func(daemon *dockerMock.MockDaemon) {
					daemon.On("CreateVolume", mock.Anything, docker.VolumeSpec{Name: "data"}).Once().Return(docker.Volume{Name: "data", Driver: "local", CreatedAt: workloadtest.At}, nil)
				},
				status: http.StatusCreated,
				answer: `{"name": "data", "driver": "local", "mountpoint": "", "in_use": false, "created_at": "2026-10-04T12:00:00Z"}`,
			},
			{
				name:   "one asked for wrongly is refused before the VM is asked",
				method: http.MethodPost,
				target: "/vms/vm-uuid/volumes",
				body:   `{}`,
				status: http.StatusBadRequest,
			},
			{
				name:   "a removal",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/volumes/data?force=true",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("DeleteVolume", mock.Anything, ownerUUID, "vm-uuid", "data", true, false).Once().Return(workloadControlPlane.Deleted{}, nil)
				},
				status: http.StatusNoContent,
			},
			{
				name:   "a removal refused, since a container mounts it, answered with it beside the refusal",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/volumes/data",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("DeleteVolume", mock.Anything, ownerUUID, "vm-uuid", "data", false, false).Once().Return(workloadControlPlane.Deleted{
						Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "app", Recreates: true}},
						Recreated:  true,
					}, &noderequest.Error{Code: noderequest.CodeInvalid, Message: "remove data: volume is in use - [c-1]"})
				},
				status: http.StatusBadRequest,
				answer: `{"errors": {"docker": "Docker refused the request: remove data: volume is in use - [c-1]"}, "dependents": [{"kind": "container", "uuid": "c-uuid", "name": "app", "recreates": true}], "recreated": true}`,
			},
			{
				name:   "and with cascade, the container goes first, and then it",
				method: http.MethodDelete,
				target: "/vms/vm-uuid/volumes/data?cascade=true",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("DeleteVolume", mock.Anything, ownerUUID, "vm-uuid", "data", false, true).Once().Return(workloadControlPlane.Deleted{
						Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "app", Recreates: true}},
					}, nil)
				},
				status: http.StatusAccepted,
				answer: `{"dependents": [{"kind": "container", "uuid": "c-uuid", "name": "app", "recreates": true}], "recreated": false}`,
			},
			{
				name:   "what needs it, read and nothing deleted",
				method: http.MethodGet,
				target: "/vms/vm-uuid/volumes/data/dependents",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("VolumeDependents", mock.Anything, ownerUUID, "vm-uuid", "data").Once().Return(workloadControlPlane.Deleted{
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
				target: "/vms/vm-uuid/volumes/data/dependents",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("VolumeDependents", mock.Anything, ownerUUID, "vm-uuid", "data").Once().Return(workloadControlPlane.Deleted{}, nil)
				},
				status: http.StatusOK,
				answer: `{"dependents": [], "recreated": false}`,
			},
			{
				name:   "and of one that is not there, or not the caller's, nothing",
				method: http.MethodGet,
				target: "/vms/vm-uuid/volumes/gone/dependents",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("VolumeDependents", mock.Anything, ownerUUID, "vm-uuid", "gone").Once().Return(workloadControlPlane.Deleted{}, domain.ErrNotExists)
				},
				status: http.StatusNotFound,
				answer: `{"code": "not_found"}`,
			},
			{
				name:   "and a workload that cannot be reached is a failure, which says nothing more",
				method: http.MethodGet,
				target: "/vms/vm-uuid/volumes/data/dependents",
				asked: func(workload *controlplane.MockClient, ownerUUID string) {
					workload.On("VolumeDependents", mock.Anything, ownerUUID, "vm-uuid", "data").Once().Return(workloadControlPlane.Deleted{}, workloadtest.ErrUnreachable)
				},
				status: http.StatusInternalServerError,
				answer: `{"code": "internal"}`,
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
