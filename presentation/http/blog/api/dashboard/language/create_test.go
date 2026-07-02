package language

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	createlanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/createLanguage"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestCreateHandler(t *testing.T) {
	t.Parallel()

	t.Run("create language", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator
			translator         translator.TranslatorMock

			u = user.User{UUID: "auth-user-uuid"}

			r = createlanguage.Request{Code: "DE", Name: "Deutsch"}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", mock.Anything, r.Code).Once().Return(false)
		languageRepository.On("Save", mock.Anything, mock.AnythingOfType("*language.Language")).Once().Return(r.Code, nil)
		defer languageRepository.AssertExpectations(t)

		handler := NewCreateHandler(createlanguage.NewUseCase(&languageRepository, &requestValidator, &translator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/create-language-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator
			translator         translator.TranslatorMock

			u = user.User{UUID: "auth-user-uuid"}

			r = createlanguage.Request{}

			validationErrors = domain.ValidationErrors{
				"code": "required_field",
				"name": "required_field",
			}
		)

		requestValidator.On("Validate", &r).Once().Return(validationErrors)
		defer requestValidator.AssertExpectations(t)

		handler := NewCreateHandler(createlanguage.NewUseCase(&languageRepository, &requestValidator, &translator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		languageRepository.AssertNotCalled(t, "Save")

		expectedBody, err := os.ReadFile("testdata/create-language-validation-errors-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
