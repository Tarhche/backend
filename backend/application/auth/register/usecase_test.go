package register

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	if err != nil {
		t.Error("unexpected error")
	}
	j := jwt.NewJWT(privateKey, privateKey.Public())

	mailFrom := "info@noreply.nowhere.loc"

	t.Run("sends registration mail", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer

			r = Request{
				Identity: "test@mail.com",
			}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, r.Identity, "Registration", mock.AnythingOfType("[]uint8")).Once().Return(nil)
		defer mailer.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom).Execute(r)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 0)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer

			r = Request{
				Identity: "somethingForTest",
			}
		)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom).Execute(r)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		mailer.AssertNotCalled(t, "SendMail")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.ValidationErrors, 1)
	})

	t.Run("user exists", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer

			r = Request{
				Identity: "test@mail.com",
			}

			u = user.User{
				Email: r.Identity,
			}

			expectedResponse = Response{
				ValidationErrors: map[string]string{
					"identity": "user with given email already exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom).Execute(r)

		mailer.AssertNotCalled(t, "SendMail")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("get user fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer

			r = Request{
				Identity: "test@mail.com",
			}

			u = user.User{
				Email: r.Identity,
			}

			expectedError = errors.New("some error")
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(u, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom).Execute(r)

		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("sending mail fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer

			r = Request{
				Identity: "test@mail.com",
			}

			expectedError = errors.New("some error")
		)

		userRepository.On("GetOneByIdentity", r.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, r.Identity, "Registration", mock.AnythingOfType("[]uint8")).Once().Return(expectedError)
		defer mailer.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom).Execute(r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
