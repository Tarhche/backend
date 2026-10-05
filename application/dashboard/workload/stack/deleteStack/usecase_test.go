package deleteStack

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		for _, removeVolumes := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, taking its volumes too: %t", scope.Name, removeVolumes), func(t *testing.T) {
				t.Parallel()

				var workload controlplane.MockClient
				workload.On("DeleteStack", mock.Anything, scope.OwnerUUID, "stack-uuid", removeVolumes).Once().Return(nil)
				defer workload.AssertExpectations(t)

				response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
					UUID:          "stack-uuid",
					RemoveVolumes: removeVolumes,
					OwnerUUID:     scope.OwnerUUID,
				})
				require.NoError(t, err)

				assert.Empty(t, response.ValidationErrors)
			})
		}
	}

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("DeleteStack", mock.Anything, workloadtest.OwnerUUID, "stack-uuid", false).Once().Return(answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				UUID:      "stack-uuid",
				OwnerUUID: workloadtest.OwnerUUID,
			})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
		})
	}
}
