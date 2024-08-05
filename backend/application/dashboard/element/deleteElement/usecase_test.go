package deleteelement

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("deletes an element", func(t *testing.T) {
		var (
			elementRepository elements.MockElementsRepository

			r = Request{ElementUUID: "element-uuid"}
		)

		elementRepository.On("Delete", r.ElementUUID).Return(nil)
		defer elementRepository.AssertExpectations(t)

		err := NewUseCase(&elementRepository).Execute(r)

		assert.NoError(t, err)
	})

	t.Run("deleting an element fails", func(t *testing.T) {
		var (
			elementRepository elements.MockElementsRepository

			r             = Request{ElementUUID: "element-uuid"}
			expectedError = errors.New("role deletion failed")
		)

		elementRepository.On("Delete", r.ElementUUID).Return(expectedError)
		defer elementRepository.AssertExpectations(t)

		err := NewUseCase(&elementRepository).Execute(r)

		assert.ErrorIs(t, err, expectedError)
	})
}
