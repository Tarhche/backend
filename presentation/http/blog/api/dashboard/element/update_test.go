package element

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	updateelement "github.com/khanzadimahdi/testproject/application/dashboard/element/updateElement"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUpdateHandler(t *testing.T) {
	t.Parallel()

	t.Run("malformed json is rejected before the use case runs", func(t *testing.T) {
		t.Parallel()

		var (
			elementRepository elements.MockElementsRepository
			requestValidator  validator.MockValidator
		)

		defer elementRepository.AssertExpectations(t)
		defer requestValidator.AssertExpectations(t)

		handler := NewUpdateHandler(updateelement.NewUseCase(&elementRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"body":}`))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
