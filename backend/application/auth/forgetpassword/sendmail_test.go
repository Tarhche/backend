package forgetpassword

import (
	"encoding/json"
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

func TestHandler_Execute(t *testing.T) {
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

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
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

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		err = NewSendForgetPasswordEmailHandler(&userRepository, j, &mailer, mailFrom, &renderer).Handle(payload)
		assert.NoError(t, err)
	})

	t.Run("user not found", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		err = NewSendForgetPasswordEmailHandler(&userRepository, j, &mailer, mailFrom, &renderer).Handle(payload)

		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		assert.NoError(t, err)
	})

	t.Run("error on finding user", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}

			expectedErr = errors.New("something bad happened")
		)

		userRepository.On("GetOneByIdentity", request.Identity).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		err = NewSendForgetPasswordEmailHandler(&userRepository, j, &mailer, mailFrom, &renderer).Handle(payload)

		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, expectedErr, err)
	})

	t.Run("error on rendering template", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
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

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		err = NewSendForgetPasswordEmailHandler(&userRepository, j, &mailer, mailFrom, &renderer).Handle(payload)

		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, expectedErr, err)
	})

	t.Run("error on sending email", func(t *testing.T) {
		var (
			userRepository users.MockUsersRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
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

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		err = NewSendForgetPasswordEmailHandler(&userRepository, j, &mailer, mailFrom, &renderer).Handle(payload)
		assert.ErrorIs(t, expectedErr, err)
	})

}
