package forgetpassword

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
	"github.com/khanzadimahdi/testproject/infrastructure/template"
)

func TestUseCase_Execute(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	mailFrom := "info@noreply.nowhere.loc"

	t.Run("successfully mails reset-password token", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}

			u = user.User{
				UUID:  "user-uuid",
				Email: request.Identity,
			}
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		renderer.On("Render", mock.Anything, templateName, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, u.Email, "Reset Password", mock.AnythingOfType("[]uint8")).Once().Return(nil)
		defer mailer.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer).Execute(request)
		assert.NoError(t, err)
		assert.NotNil(t, response)
	})

	t.Run("validation fails", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request          = Request{}
			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"identity": "identity is required",
				},
			}
		)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer).Execute(request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("user not found", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}
			expectedResponse = Response{
				ValidationErrors: validationErrors{
					"identity": "identity (email/username) not exists",
				},
			}
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer).Execute(request)

		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("error on finding user", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}

			expectedErr = errors.New("something bad happened")
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer).Execute(request)

		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

	t.Run("error on rendering template", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}

			u = user.User{
				UUID:  "user-uuid",
				Email: request.Identity,
			}

			expectedErr = errors.New("something bad happened")
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		renderer.On("Render", mock.Anything, templateName, mock.Anything).Once().Return(expectedErr)
		defer renderer.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer).Execute(request)

		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

	t.Run("error on sending email", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}

			u = user.User{
				UUID:  "user-uuid",
				Email: request.Identity,
			}

			expectedErr = errors.New("something bad happened")
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		renderer.On("Render", mock.Anything, templateName, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, u.Email, "Reset Password", mock.AnythingOfType("[]uint8")).Once().Return(expectedErr)
		defer mailer.AssertExpectations(t)

		response, err := NewUseCase(&userRepository, j, &mailer, mailFrom, &renderer).Execute(request)
		assert.ErrorIs(t, expectedErr, err)
		assert.Nil(t, response)
	})

}
