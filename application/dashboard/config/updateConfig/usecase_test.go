package updateConfig

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	mock2 "github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	var translatorOptionsType = reflect.TypeFor[func(*translatorContract.Params)]().Name()

	t.Run("updates config", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository   configMocks.MockConfigRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				UserDefaultRoles:    []string{"role1", "role2"},
				DefaultLanguageCode: "EN",
			}

			loadedConfig = config.Config{
				Revision:             1,
				UserDefaultRoleUUIDs: []string{"role1"},
			}

			savedConfig = config.Config{
				Revision:             loadedConfig.Revision,
				UserDefaultRoleUUIDs: r.UserDefaultRoles,
				DefaultLanguageCode:  r.DefaultLanguageCode,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", r.DefaultLanguageCode).Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(loadedConfig, nil)
		configRepository.On("Save", &savedConfig).Once().Return("new-revision-uuid", nil)
		defer configRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &languageRepository, &validator, &translator).Execute(&r)
		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository   configMocks.MockConfigRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"user_default_roles":    "user_default_roles is required",
					"default_language_code": "default_language_code is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &languageRepository, &validator, &translator).Execute(&r)

		languageRepository.AssertNotCalled(t, "Exists")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		configRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("default language does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository   configMocks.MockConfigRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				UserDefaultRoles:    []string{"role1", "role2"},
				DefaultLanguageCode: "DE",
			}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"default_language_code": "default language code is invalid",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		translator.On(
			"Translate",
			"invalid_value",
			mock2.AnythingOfType(translatorOptionsType),
		).Once().Return(expectedResponse.ValidationErrors["default_language_code"])
		defer translator.AssertExpectations(t)

		languageRepository.On("Exists", r.DefaultLanguageCode).Once().Return(false)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &languageRepository, &validator, &translator).Execute(&r)

		configRepository.AssertNotCalled(t, "GetLatestRevision")
		configRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("get last revision of config fails", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository   configMocks.MockConfigRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				UserDefaultRoles:    []string{"role1", "role2"},
				DefaultLanguageCode: "EN",
			}

			expectedErr = errors.New("error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", r.DefaultLanguageCode).Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(config.Config{}, expectedErr)
		defer configRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &languageRepository, &validator, &translator).Execute(&r)

		configRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("saving config fails", func(t *testing.T) {
		t.Parallel()

		var (
			configRepository   configMocks.MockConfigRepository
			languageRepository languages.MockLanguagesRepository
			validator          validator.MockValidator
			translator         translator.TranslatorMock

			r = Request{
				UserDefaultRoles:    []string{"role1", "role2"},
				DefaultLanguageCode: "EN",
			}

			loadedConfig = config.Config{
				Revision:             1,
				UserDefaultRoleUUIDs: []string{"role1"},
			}

			savedConfig = config.Config{
				Revision:             loadedConfig.Revision,
				UserDefaultRoleUUIDs: r.UserDefaultRoles,
				DefaultLanguageCode:  r.DefaultLanguageCode,
			}

			expectedErr = errors.New("error")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageRepository.On("Exists", r.DefaultLanguageCode).Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		configRepository.On("GetLatestRevision").Once().Return(loadedConfig, nil)
		configRepository.On("Save", &savedConfig).Once().Return("", expectedErr)
		defer configRepository.AssertExpectations(t)

		response, err := NewUseCase(&configRepository, &languageRepository, &validator, &translator).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
