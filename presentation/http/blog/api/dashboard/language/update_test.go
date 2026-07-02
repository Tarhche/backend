package language

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	updatelanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/updateLanguage"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUpdateHandler(t *testing.T) {
	t.Parallel()

	t.Run("update language", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}

			r = updatelanguage.Request{Code: "EN", Name: "English (US)"}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", mock.Anything, r.Code).Once().Return(true)
		languageRepository.On("Save", mock.Anything, mock.AnythingOfType("*language.Language")).Once().Return(r.Code, nil)
		defer languageRepository.AssertExpectations(t)

		handler := NewUpdateHandler(updatelanguage.NewUseCase(&languageRepository, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("language does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			u = user.User{UUID: "auth-user-uuid"}

			r = updatelanguage.Request{Code: "DE", Name: "Deutsch"}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", mock.Anything, r.Code).Once().Return(false)
		defer languageRepository.AssertExpectations(t)

		handler := NewUpdateHandler(updatelanguage.NewUseCase(&languageRepository, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPut, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		languageRepository.AssertNotCalled(t, "Save")
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
