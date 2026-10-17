package snapshot

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/createSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/deleteSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshots"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/renameSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
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

	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/snapshots", NewCreateHandler(createSnapshot.NewUseCase(workload, v, t, owners)))

	for _, set := range workloadhttptest.Sets {
		mux.Handle("GET "+set.Prefix+"/snapshots", NewIndexHandler(getSnapshots.NewUseCase(workload, owners), set.Owner))
		mux.Handle("GET "+set.Prefix+"/snapshots/{uuid}", NewShowHandler(getSnapshot.NewUseCase(workload, owners), set.Owner))
		mux.Handle("PATCH "+set.Prefix+"/snapshots/{uuid}", NewUpdateHandler(renameSnapshot.NewUseCase(workload, v, t, owners), set.Owner))
		mux.Handle("DELETE "+set.Prefix+"/snapshots/{uuid}", NewDeleteHandler(deleteSnapshot.NewUseCase(workload, t), set.Owner))
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
			expect func(workload *controlplane.MockClient)
			status int
			answer string
		}{
			{
				name:   "a page of one VM's snapshots",
				method: http.MethodGet,
				target: "/snapshots?vm=vm-uuid&page=3",
				expect: func(workload *controlplane.MockClient) {
					workload.On("Snapshots", mock.Anything, set.OwnerUUID, "vm-uuid", uint(3)).Once().Return(workloadControlPlane.Page[snapshot.Snapshot]{
						Items: []snapshot.Snapshot{workloadtest.Snapshot()}, TotalPages: 3, CurrentPage: 3,
					}, nil)
				},
				status: http.StatusOK,
				answer: `{"items": [` + workloadtest.SnapshotJSON + `], "pagination": {"total_pages": 3, "current_page": 3}}`,
			},
			{
				name:   "one snapshot",
				method: http.MethodGet,
				target: "/snapshots/snapshot-uuid",
				expect: func(workload *controlplane.MockClient) {
					workload.On("Snapshot", mock.Anything, set.OwnerUUID, "snapshot-uuid").Once().Return(workloadtest.Snapshot(), nil)
				},
				status: http.StatusOK,
				answer: workloadtest.SnapshotJSON,
			},
			{
				name:   "one that is not there, or not the caller's",
				method: http.MethodGet,
				target: "/snapshots/gone-uuid",
				expect: func(workload *controlplane.MockClient) {
					workload.On("Snapshot", mock.Anything, set.OwnerUUID, "gone-uuid").Once().Return(snapshot.Snapshot{}, domain.ErrNotExists)
				},
				status: http.StatusNotFound,
				answer: `{"code": "not_found"}`,
			},
			{
				name:   "a rename",
				method: http.MethodPatch,
				target: "/snapshots/snapshot-uuid",
				body:   `{"name": "before the upgrade"}`,
				expect: func(workload *controlplane.MockClient) {
					workload.On("RenameSnapshot", mock.Anything, set.OwnerUUID, "snapshot-uuid", "before the upgrade").Once().Return(workloadtest.Snapshot(), nil)
				},
				status: http.StatusOK,
				answer: workloadtest.SnapshotJSON,
			},
			{
				name:   "a rename to nothing",
				method: http.MethodPatch,
				target: "/snapshots/snapshot-uuid",
				body:   `{"name": ""}`,
				status: http.StatusBadRequest,
				answer: `{"errors": {"name": "this field is required"}}`,
			},
			{
				name:   "a delete, done once it answers",
				method: http.MethodDelete,
				target: "/snapshots/snapshot-uuid",
				expect: func(workload *controlplane.MockClient) {
					workload.On("DeleteSnapshot", mock.Anything, set.OwnerUUID, "snapshot-uuid").Once().Return(nil)
				},
				status: http.StatusNoContent,
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

	t.Run("a snapshot is taken for whoever asks, of the VM in the path", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateSnapshot", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "before the upgrade").Once().Return(workloadtest.Snapshot(), nil)
		defer workload.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&workload), http.MethodPost, "/api/dashboard/workload/vms/vm-uuid/snapshots", `{"name": "before the upgrade"}`)

		assert.Equal(t, http.StatusCreated, response.Code)
		assert.JSONEq(t, workloadtest.SnapshotJSON, response.Body.String())
	})

	t.Run("what the control plane refuses is said by field", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateSnapshot", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "one too many").Once().Return(
			snapshot.Snapshot{},
			&client.ValidationError{ValidationErrors: domain.ValidationErrors{"name": "quota_exceeded"}},
		)
		defer workload.AssertExpectations(t)

		response := workloadhttptest.Serve(routes(&workload), http.MethodPost, "/api/dashboard/workload/vms/vm-uuid/snapshots", `{"name": "one too many"}`)

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(t, `{"errors": {"name": "this would take you past your quota"}}`, response.Body.String())
	})
}
