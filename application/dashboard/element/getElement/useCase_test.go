package getelement

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("gets an element", func(t *testing.T) {
		t.Parallel()

		var (
			elementRepository elements.MockElementsRepository
			mockComponent     component.MockComponent
		)

		var (
			a = element.Element{
				UUID:      "element-uuid-1",
				Type:      "item",
				Body:      &mockComponent,
				Venues:    []string{},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		)

		elementRepository.On("GetOne", a.UUID).Return(a, nil)
		defer elementRepository.AssertExpectations(t)

		response, err := NewUseCase(&elementRepository).Execute(a.UUID)

		assert.NoError(t, err)
		assert.NotNil(t, response)
	})

	t.Run("getting an element fails", func(t *testing.T) {
		t.Parallel()

		var (
			elementRepository elements.MockElementsRepository

			elementUUID   = "element-uuid"
			expectedError = errors.New("error")
		)

		elementRepository.On("GetOne", elementUUID).Once().Return(element.Element{}, expectedError)
		defer elementRepository.AssertExpectations(t)

		response, err := NewUseCase(&elementRepository).Execute(elementUUID)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
