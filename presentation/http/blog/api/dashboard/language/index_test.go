package language

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	getlanguages "github.com/khanzadimahdi/testproject/application/dashboard/language/getLanguages"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("languages", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			u = user.User{UUID: "auth-user-uuid"}

			l = []language.Language{
				{Code: "EN", Name: "English"},
				{Code: "FA", Name: "فارسی"},
			}
		)

		languageRepository.On("Count", mock.Anything).Once().Return(uint(len(l)), nil)
		languageRepository.On("GetAll", mock.Anything, uint(0), uint(10)).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		handler := NewIndexHandler(getlanguages.NewUseCase(&languageRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-languages-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})
}
