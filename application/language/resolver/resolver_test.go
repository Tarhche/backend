package resolver

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/language"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestResolver_DefaultCode(t *testing.T) {
	t.Parallel()

	t.Run("returns the configured default", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			configRepository   configMocks.MockConfigRepository
		)

		configRepository.On("GetLatestRevision").Once().Return(config.Config{DefaultLanguageCode: "FA"}, nil)
		defer configRepository.AssertExpectations(t)

		code, err := New(&languageRepository, &configRepository).DefaultCode()

		languageRepository.AssertNotCalled(t, "GetOne")
		assert.NoError(t, err)
		assert.Equal(t, "FA", code)
	})

	t.Run("errors when no default is configured", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			configRepository   configMocks.MockConfigRepository
		)

		configRepository.On("GetLatestRevision").Once().Return(config.Config{}, domain.ErrNotExists)
		defer configRepository.AssertExpectations(t)

		code, err := New(&languageRepository, &configRepository).DefaultCode()

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, code)
	})
}

func TestResolver_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("fetches the requested language without substituting a default", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			configRepository   configMocks.MockConfigRepository
		)

		languageRepository.On("GetOne", "FA").Once().Return(language.Language{Code: "FA", Name: "فارسی"}, nil)
		defer languageRepository.AssertExpectations(t)

		lang, err := New(&languageRepository, &configRepository).Resolve("FA")

		languageRepository.AssertNotCalled(t, "Exists")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		assert.NoError(t, err)
		assert.Equal(t, "FA", lang.Code)
	})

	t.Run("propagates not-found errors", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			configRepository   configMocks.MockConfigRepository
		)

		languageRepository.On("GetOne", "DE").Once().Return(language.Language{}, domain.ErrNotExists)
		defer languageRepository.AssertExpectations(t)

		_, err := New(&languageRepository, &configRepository).Resolve("DE")

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("propagates unexpected errors", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			configRepository   configMocks.MockConfigRepository

			expectedErr = errors.New("db error")
		)

		languageRepository.On("GetOne", "EN").Once().Return(language.Language{}, expectedErr)
		defer languageRepository.AssertExpectations(t)

		_, err := New(&languageRepository, &configRepository).Resolve("EN")

		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestResolver_Verify(t *testing.T) {
	t.Parallel()

	t.Run("acceptable when the language exists", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			configRepository   configMocks.MockConfigRepository
		)

		languageRepository.On("Exists", "FA").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		valid := New(&languageRepository, &configRepository).Verify("FA")

		languageRepository.AssertNotCalled(t, "GetOne")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		assert.True(t, valid)
	})

	t.Run("not acceptable when the language does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			configRepository   configMocks.MockConfigRepository
		)

		languageRepository.On("Exists", "DE").Once().Return(false)
		defer languageRepository.AssertExpectations(t)

		valid := New(&languageRepository, &configRepository).Verify("DE")

		languageRepository.AssertNotCalled(t, "GetOne")
		configRepository.AssertNotCalled(t, "GetLatestRevision")
		assert.False(t, valid)
	})
}
