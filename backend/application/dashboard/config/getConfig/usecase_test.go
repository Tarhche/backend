package getConfig

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/config"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("gets config", func(t *testing.T) {
		var (
			configRepository configMocks.MockConfigRepository

			loadedConfig = config.Config{
				Revision:             1,
				UserDefaultRoleUUIDs: []string{"role1"},
			}

			expectedResponse = Response{
				Revision:         loadedConfig.Revision,
				UserDefaultRoles: loadedConfig.UserDefaultRoleUUIDs,
			}
		)

		configRepository.On("GetLatestRevision").Once().Return(loadedConfig, nil)
		defer configRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository).Execute()
		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("get last revision of config fails", func(t *testing.T) {
		var (
			configRepository configMocks.MockConfigRepository

			expectedErr = errors.New("error")
		)

		configRepository.On("GetLatestRevision").Once().Return(config.Config{}, expectedErr)
		defer configRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository).Execute()

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
