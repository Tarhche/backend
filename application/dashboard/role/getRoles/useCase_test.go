package getroles

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting roles", func(t *testing.T) {
		t.Parallel()

		var (
			rolesRepository roles.MockRolesRepository

			r = Request{
				Page: 0,
			}

			p = []role.Role{
				{
					UUID:        "test-uuid-01",
					Name:        "role-name-01",
					Description: "test role description-01",
					Permissions: []string{"permission-1"},
					UserUUIDs:   []string{"user-uuid-1"},
				},
				{
					UUID: "test-uuid-02",
					Name: "role-name-02",
				},
				{Name: "role-name-03"},
			}

			expectedResponse = Response{
				Items: []roleResponse{
					{
						UUID:        p[0].UUID,
						Name:        p[0].Name,
						Description: p[0].Description,
					},
					{
						UUID: p[1].UUID,
						Name: p[1].Name,
					},
					{
						Name: p[2].Name,
					},
				},
				Pagination: pagination{
					TotalPages:  1,
					CurrentPage: 1,
				},
			}
		)

		rolesRepository.On("Count").Once().Return(uint(len(p)), nil)
		rolesRepository.On("GetAll", uint(0), uint(10)).Once().Return(p, nil)
		defer rolesRepository.AssertExpectations(t)

		response, err := NewUseCase(&rolesRepository).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure on counting roles", func(t *testing.T) {
		t.Parallel()

		var (
			rolesRepository roles.MockRolesRepository

			r = Request{
				Page: 0,
			}

			expectedError = errors.New("error")
		)

		rolesRepository.On("Count").Once().Return(uint(0), expectedError)
		defer rolesRepository.AssertExpectations(t)

		response, err := NewUseCase(&rolesRepository).Execute(&r)

		rolesRepository.AssertNotCalled(t, "GetAll")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("failure on getting roles", func(t *testing.T) {
		t.Parallel()

		var (
			rolesRepository roles.MockRolesRepository

			r = Request{
				Page: 0,
			}

			expectedError = errors.New("error")
		)

		rolesRepository.On("Count").Once().Return(uint(3), nil)
		rolesRepository.On("GetAll", uint(0), uint(10)).Once().Return(nil, expectedError)
		defer rolesRepository.AssertExpectations(t)

		response, err := NewUseCase(&rolesRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
