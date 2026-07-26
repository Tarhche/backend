package element

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	createelement "github.com/khanzadimahdi/testproject/application/dashboard/element/createElement"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestCreateHandler(t *testing.T) {
	t.Parallel()

	t.Run("malformed json is rejected before the use case runs", func(t *testing.T) {
		t.Parallel()

		var (
			elementRepository elements.MockElementsRepository
			requestValidator  validator.MockValidator
		)

		defer elementRepository.AssertExpectations(t)
		defer requestValidator.AssertExpectations(t)

		handler := NewCreateHandler(createelement.NewUseCase(&elementRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"body":}`))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
