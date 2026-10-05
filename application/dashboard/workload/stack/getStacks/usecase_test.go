package getStacks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Stacks", mock.Anything, scope.OwnerUUID, "vm-uuid", uint(1)).Once().Return(workloadControlPlane.Page[stack.Stack]{
				Items:       []stack.Stack{workloadtest.Stack()},
				TotalPages:  1,
				CurrentPage: 1,
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			// a listing is read to choose one, so the compose files stay out
			// of it.
			assert.JSONEq(t, `{
				"items": [{
					"uuid": "stack-uuid",
					"name": "shop",
					"slug": "shop-abcde",
					"owner_uuid": "owner-uuid",
					"owner": {"uuid": "owner-uuid", "name": "Mahdi", "username": "mahdi", "avatar": "avatar-uuid"},
					"vm_uuid": "vm-uuid",
					"vm_name": "docker-1",
					"state": "running",
					"expected_state": "running",
					"output": "Container shop-abcde-web-1  Started",
					"created_at": "2026-10-04T12:00:00Z"
				}],
				"pagination": {"total_pages": 1, "current_page": 1}
			}`, string(presented))
		})
	}

	for _, answer := range workloadtest.Failures() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Stacks", mock.Anything, workloadtest.OwnerUUID, "", uint(3)).Once().Return(workloadControlPlane.Page[stack.Stack]{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{
				Page:      3,
				OwnerUUID: workloadtest.OwnerUUID,
			})

			assert.ErrorIs(t, err, answer.Failure)
			assert.Nil(t, response)
		})
	}
}
