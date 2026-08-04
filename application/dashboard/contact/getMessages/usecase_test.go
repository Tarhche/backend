package getMessages

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/contact"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting messages succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			readAt    = time.Now()
			createdAt = time.Now()

			m = []contact.Message{
				{
					UUID:      "message-uuid-1",
					Subject:   "subject-1",
					Body:      "body-1",
					Email:     "user@example.com",
					CreatedAt: createdAt,
				},
				{
					UUID:      "message-uuid-2",
					Subject:   "subject-2",
					Body:      "body-2",
					Phone:     "09123456789",
					ReadAt:    readAt,
					CreatedAt: createdAt,
				},
			}

			r = Request{
				Page: 0,
			}

			expectedResponse = Response{
				Items: []messageResponse{
					{
						UUID:      m[0].UUID,
						Subject:   m[0].Subject,
						Body:      m[0].Body,
						Email:     m[0].Email,
						ReadAt:    m[0].ReadAt.Format(time.RFC3339),
						CreatedAt: m[0].CreatedAt.Format(time.RFC3339),
					},
					{
						UUID:      m[1].UUID,
						Subject:   m[1].Subject,
						Body:      m[1].Body,
						Phone:     m[1].Phone,
						ReadAt:    m[1].ReadAt.Format(time.RFC3339),
						CreatedAt: m[1].CreatedAt.Format(time.RFC3339),
					},
				},
				Pagination: pagination{
					CurrentPage: 1,
					TotalPages:  1,
				},
			}
		)

		contactRepository.On("Count", mock.Anything).Once().Return(uint(len(m)), nil)
		contactRepository.On("GetAll", mock.Anything, uint(0), uint(10)).Once().Return(m, nil)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("counting messages fails", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			r = Request{
				Page: 0,
			}

			expectedErr = errors.New("counting messages failed")
		)

		contactRepository.On("Count", mock.Anything).Once().Return(uint(0), expectedErr)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		contactRepository.AssertNotCalled(t, "GetAll")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting messages fails", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository

			r = Request{
				Page: 2,
			}

			expectedErr = errors.New("getting messages failed")
		)

		contactRepository.On("Count", mock.Anything).Once().Return(uint(12), nil)
		contactRepository.On("GetAll", mock.Anything, uint(10), uint(10)).Once().Return(nil, expectedErr)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
