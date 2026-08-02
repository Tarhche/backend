package markAsRead

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

	t.Run("marking an unread message as read stamps the read time", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			m = contact.Message{
				UUID:      "message-uuid",
				Subject:   "a subject",
				Body:      "a body",
				CreatedAt: time.Now(),
			}

			r = Request{
				MessageUUID: m.UUID,
				Read:        true,
			}
		)

		contactRepository.On("GetOne", mock.Anything, m.UUID).Once().Return(m, nil)
		contactRepository.On("Save", mock.Anything, mock.MatchedBy(func(saved *contact.Message) bool {
			return saved.UUID == m.UUID && saved.IsRead()
		})).Once().Return(m.UUID, nil)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.Equal(t, m.UUID, response.UUID)
		assert.NotEqual(t, time.Time{}.Format(time.RFC3339), response.ReadAt)
	})

	t.Run("marking an already read message keeps the original read time", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			readAt = time.Now().Add(-24 * time.Hour)

			m = contact.Message{
				UUID:   "message-uuid",
				ReadAt: readAt,
			}

			r = Request{
				MessageUUID: m.UUID,
				Read:        true,
			}
		)

		contactRepository.On("GetOne", mock.Anything, m.UUID).Once().Return(m, nil)
		contactRepository.On("Save", mock.Anything, mock.MatchedBy(func(saved *contact.Message) bool {
			return saved.ReadAt.Equal(readAt)
		})).Once().Return(m.UUID, nil)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.Equal(t, readAt.Format(time.RFC3339), response.ReadAt)
	})

	t.Run("marking a message as unread clears the read time", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			m = contact.Message{
				UUID:   "message-uuid",
				ReadAt: time.Now(),
			}

			r = Request{
				MessageUUID: m.UUID,
				Read:        false,
			}
		)

		contactRepository.On("GetOne", mock.Anything, m.UUID).Once().Return(m, nil)
		contactRepository.On("Save", mock.Anything, mock.MatchedBy(func(saved *contact.Message) bool {
			return !saved.IsRead()
		})).Once().Return(m.UUID, nil)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.Equal(t, time.Time{}.Format(time.RFC3339), response.ReadAt)
	})

	t.Run("message does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			r = Request{
				MessageUUID: "message-uuid",
				Read:        true,
			}
		)

		contactRepository.On("GetOne", mock.Anything, r.MessageUUID).Once().Return(contact.Message{}, domain.ErrNotExists)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		contactRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Nil(t, response)
	})
}
