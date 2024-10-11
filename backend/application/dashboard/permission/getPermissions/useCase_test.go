package getpermissions

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/permissions"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("getting permissions", func(t *testing.T) {
		var (
			permissionsRepository permissions.MockPermissionsRepository

			p = []permission.Permission{
				{Name: "permission-name-01", Value: "permission-value-01"},
				{Name: "permission-name-02", Value: "permission-value-02"},
				{Name: "permission-name-03", Value: "permission-value-03"},
			}

			expectedResponse = Response{
				Items: []permissionResponse{
					{Name: p[0].Name, Value: p[0].Value},
					{Name: p[1].Name, Value: p[1].Value},
					{Name: p[2].Name, Value: p[2].Value},
				},
			}
		)

		permissionsRepository.On("GetAll").Once().Return(p)
		defer permissionsRepository.AssertExpectations(t)

		response, err := NewUseCase(&permissionsRepository).Execute()

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})
}
