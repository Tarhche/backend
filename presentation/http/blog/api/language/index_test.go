package language

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	getlanguages "github.com/khanzadimahdi/testproject/application/language/getLanguages"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("languages", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			l = []language.Language{
				{Code: "EN", Name: "English"},
				{Code: "FA", Name: "فارسی"},
			}
		)

		languageRepository.On("Count").Once().Return(uint(len(l)), nil)
		languageRepository.On("GetAll", uint(0), uint(len(l))).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		handler := NewIndexHandler(getlanguages.NewUseCase(&languageRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-languages-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("getting languages fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
		)

		languageRepository.On("Count").Once().Return(uint(0), assert.AnError)
		defer languageRepository.AssertExpectations(t)

		handler := NewIndexHandler(getlanguages.NewUseCase(&languageRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
