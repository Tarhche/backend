package language

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	getlanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/getLanguage"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show language", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			u = user.User{UUID: "auth-user-uuid"}
			l = language.Language{Code: "EN", Name: "English"}
		)

		languageRepository.On("GetOne", mock.Anything, l.Code).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		handler := NewShowHandler(getlanguage.NewUseCase(&languageRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("code", l.Code)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-language-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("language does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			u = user.User{UUID: "auth-user-uuid"}
		)

		languageRepository.On("GetOne", mock.Anything, "DE").Once().Return(language.Language{}, domain.ErrNotExists)
		defer languageRepository.AssertExpectations(t)

		handler := NewShowHandler(getlanguage.NewUseCase(&languageRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("code", "DE")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
