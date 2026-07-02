package forgetpassword

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/template"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
)

func TestHandler_Execute(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	mailFrom := "info@noreply.nowhere.loc"
	webURL := "https://tarhche.com"

	const (
		languageCode = "en"
		subject      = "Reset Password"
	)
	localizedTemplate := templateName + "." + languageCode

	t.Run("successfully mails reset-password token", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer
			translatorMock translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}

			u = user.User{
				UUID:         "user-uuid",
				Email:        request.Identity,
				LanguageCode: languageCode,
			}
		)

		userRepository.On("GetOneByIdentity", mock.Anything, request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		renderer.On("Render", mock.Anything, localizedTemplate, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		translatorMock.On("Translate", resetPasswordEmailSubject, mock.Anything).Once().Return(subject)
		defer translatorMock.AssertExpectations(t)

		mailer.On("SendMail", mock.Anything, mailFrom, u.Email, subject, mock.AnythingOfType("[]uint8")).Once().Return(nil)
		defer mailer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		authTokenGenerator := auth.NewTokenGenerator(j, &roleRepository)

		err = NewSendForgetPasswordEmailHandler(&userRepository, authTokenGenerator, &mailer, mailFrom, webURL, &renderer, &translatorMock).Handle(context.Background(), payload)
		assert.NoError(t, err)
	})

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer
			translatorMock translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}
		)

		userRepository.On("GetOneByIdentity", mock.Anything, request.Identity).Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		authTokenGenerator := auth.NewTokenGenerator(j, &roleRepository)

		err = NewSendForgetPasswordEmailHandler(&userRepository, authTokenGenerator, &mailer, mailFrom, webURL, &renderer, &translatorMock).Handle(context.Background(), payload)

		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		assert.NoError(t, err)
	})

	t.Run("error on finding user", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer
			translatorMock translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}

			expectedErr = errors.New("something bad happened")
		)

		userRepository.On("GetOneByIdentity", mock.Anything, request.Identity).Once().Return(user.User{}, expectedErr)
		defer userRepository.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		authTokenGenerator := auth.NewTokenGenerator(j, &roleRepository)

		err = NewSendForgetPasswordEmailHandler(&userRepository, authTokenGenerator, &mailer, mailFrom, webURL, &renderer, &translatorMock).Handle(context.Background(), payload)

		renderer.AssertNotCalled(t, "Render")
		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, expectedErr, err)
	})

	t.Run("error on rendering template", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer
			translatorMock translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}

			u = user.User{
				UUID:         "user-uuid",
				Email:        request.Identity,
				LanguageCode: languageCode,
			}

			expectedErr = errors.New("something bad happened")
		)

		userRepository.On("GetOneByIdentity", mock.Anything, request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		renderer.On("Render", mock.Anything, localizedTemplate, mock.Anything).Once().Return(expectedErr)
		defer renderer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		authTokenGenerator := auth.NewTokenGenerator(j, &roleRepository)

		err = NewSendForgetPasswordEmailHandler(&userRepository, authTokenGenerator, &mailer, mailFrom, webURL, &renderer, &translatorMock).Handle(context.Background(), payload)

		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, expectedErr, err)
	})

	t.Run("error on sending email", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository
			roleRepository roles.MockRolesRepository
			mailer         email.MockMailer
			renderer       template.MockRenderer
			translatorMock translator.TranslatorMock

			request = Request{
				Identity: "something@somewhere.loc",
			}

			command = SendForgetPasswordEmail{
				Identity: request.Identity,
			}

			u = user.User{
				UUID:         "user-uuid",
				Email:        request.Identity,
				LanguageCode: languageCode,
			}

			expectedErr = errors.New("something bad happened")
		)

		userRepository.On("GetOneByIdentity", mock.Anything, request.Identity).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		renderer.On("Render", mock.Anything, localizedTemplate, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		translatorMock.On("Translate", resetPasswordEmailSubject, mock.Anything).Once().Return(subject)
		defer translatorMock.AssertExpectations(t)

		mailer.On("SendMail", mock.Anything, mailFrom, u.Email, subject, mock.AnythingOfType("[]uint8")).Once().Return(expectedErr)
		defer mailer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		authTokenGenerator := auth.NewTokenGenerator(j, &roleRepository)

		err = NewSendForgetPasswordEmailHandler(&userRepository, authTokenGenerator, &mailer, mailFrom, webURL, &renderer, &translatorMock).Handle(context.Background(), payload)
		assert.ErrorIs(t, expectedErr, err)
	})
}
