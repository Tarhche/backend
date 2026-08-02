package deleteMessage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deleting a message succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			r = Request{
				MessageUUID: "message-uuid",
			}
		)

		contactRepository.On("Delete", mock.Anything, r.MessageUUID).Once().Return(nil)
		defer contactRepository.AssertExpectations(t)

		err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
	})

	t.Run("deleting a message fails", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			r = Request{
				MessageUUID: "message-uuid",
			}

			expectedErr = errors.New("deleting the message failed")
		)

		contactRepository.On("Delete", mock.Anything, r.MessageUUID).Once().Return(expectedErr)
		defer contactRepository.AssertExpectations(t)

		err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedErr)
	})
}
