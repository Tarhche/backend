package language

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	deletelanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/deleteLanguage"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete language", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			u = user.User{UUID: "auth-user-uuid"}

			r = deletelanguage.Request{Code: "EN"}
		)

		languageRepository.On("Delete", mock.Anything, r.Code).Once().Return(nil)
		defer languageRepository.AssertExpectations(t)

		handler := NewDeleteHandler(deletelanguage.NewUseCase(&languageRepository))

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("code", r.Code)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
