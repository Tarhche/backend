package createMessage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/contact"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/contacts"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("sends a message", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository
			validator         validator.MockValidator

			r = Request{
				Subject: "a subject",
				Body:    "a body",
				Email:   "user@example.com",
				Phone:   "09123456789",
			}

			m = contact.Message{
				Subject: r.Subject,
				Body:    r.Body,
				Email:   r.Email,
				Phone:   r.Phone,
			}
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		contactRepository.On("Save", mock.Anything, &m).Once().Return("message-uuid", nil)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository, &validator).Execute(context.Background(), &r)

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository
			validator         validator.MockValidator

			r                = Request{}
			expectedResponse = Response{
				ValidationErrors: map[string]string{
					"subject": "this field is required",
					"body":    "this field is required",
					"email":   "either an email address or a phone number is required",
					"phone":   "either an email address or a phone number is required",
				},
			}
		)

		validator.On("Validate", &r).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository, &validator).Execute(context.Background(), &r)

		contactRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("saving the message fails", func(t *testing.T) {
		t.Parallel()

		var (
			contactRepository contacts.MockContactsRepository
			validator         validator.MockValidator

			r = Request{
				Subject: "a subject",
				Body:    "a body",
				Email:   "user@example.com",
			}

			m = contact.Message{
				Subject: r.Subject,
				Body:    r.Body,
				Email:   r.Email,
			}

			expectedErr = errors.New("saving the message failed")
		)

		validator.On("Validate", &r).Once().Return(nil)
		defer validator.AssertExpectations(t)

		contactRepository.On("Save", mock.Anything, &m).Once().Return("", expectedErr)
		defer contactRepository.AssertExpectations(t)

		response, err := NewUseCase(&contactRepository, &validator).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
