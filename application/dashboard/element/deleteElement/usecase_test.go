package deleteelement

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deletes an element", func(t *testing.T) {
		t.Parallel()

		var (
			elementRepository elements.MockElementsRepository

			r = Request{ElementUUID: "element-uuid"}
		)

		elementRepository.On("Delete", mock.Anything, r.ElementUUID).Return(nil)
		defer elementRepository.AssertExpectations(t)

		err := NewUseCase(&elementRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
	})

	t.Run("deleting an element fails", func(t *testing.T) {
		t.Parallel()

		var (
			elementRepository elements.MockElementsRepository

			r             = Request{ElementUUID: "element-uuid"}
			expectedError = errors.New("role deletion failed")
		)

		elementRepository.On("Delete", mock.Anything, r.ElementUUID).Return(expectedError)
		defer elementRepository.AssertExpectations(t)

		err := NewUseCase(&elementRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
	})
}
