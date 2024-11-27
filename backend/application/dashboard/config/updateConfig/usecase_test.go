package updateConfig

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("updates config", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository
			validator        validator.MockValidator

			r = Request{
				UserDefaultRoles: []string{"role1", "role2"},
			}

			loadedConfig = config.Config{
				Revision:             1,
				UserDefaultRoleUUIDs: []string{"role1"},
			}

			savedConfig = config.Config{
				Revision:             loadedConfig.Revision,
				UserDefaultRoleUUIDs: r.UserDefaultRoles,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(loadedConfig, nil)
		configRepository.On("Save", &savedConfig).Once().Return("new-revision-uuid", nil)
		defer configRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &validator).Execute(&r)
		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository
			validator        validator.MockValidator

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"user_default_roles": "user_default_roles is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &validator).Execute(&r)

		configRepository.AssertNotCalled(t, "GetLatestRevision")
		configRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("get last revision of config fails", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository
			validator        validator.MockValidator

			r = Request{
				UserDefaultRoles: []string{"role1", "role2"},
			}

			expectedErr = errors.New("error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(config.Config{}, expectedErr)
		defer configRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &validator).Execute(&r)

		configRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving config fails", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository configMocks.MockConfigRepository
			validator        validator.MockValidator

			r = Request{
				UserDefaultRoles: []string{"role1", "role2"},
			}

			loadedConfig = config.Config{
				Revision:             1,
				UserDefaultRoleUUIDs: []string{"role1"},
			}

			savedConfig = config.Config{
				Revision:             loadedConfig.Revision,
				UserDefaultRoleUUIDs: r.UserDefaultRoles,
			}

			expectedErr = errors.New("error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(loadedConfig, nil)
		configRepository.On("Save", &savedConfig).Once().Return("", expectedErr)
		defer configRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &validator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
