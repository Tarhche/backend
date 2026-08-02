package getMessage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/contact"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting a message succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			m = contact.Message{
				UUID:      "message-uuid",
				Subject:   "a subject",
				Body:      "a body",
				Email:     "user@example.com",
				Phone:     "09123456789",
				CreatedAt: time.Now(),
			}

			expectedResponse = Response{
				UUID:      m.UUID,
				Subject:   m.Subject,
				Body:      m.Body,
				Email:     m.Email,
				Phone:     m.Phone,
				ReadAt:    m.ReadAt.Format(time.RFC3339),
				CreatedAt: m.CreatedAt.Format(time.RFC3339),
			}
		)

		contactRepository.On("GetOne", mock.Anything, m.UUID).Once().Return(m, nil)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), m.UUID)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("message does not exist", func(t *testing.T) {
		t.Parallel()

		var contactRepository contacts.MockContactsRepository

		contactRepository.On("GetOne", mock.Anything, "message-uuid").Once().Return(contact.Message{}, domain.ErrNotExists)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), "message-uuid")

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Nil(t, response)
	})
}
