package register

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/template"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
)

func TestHandler_Execute(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	mailFrom := "info@noreply.nowhere.loc"

	const (
		languageCode = "en"
		subject      = "Registration"
	)
	localizedTemplate := templateName + "." + languageCode

	t.Run("sends registration mail", func(t *testing.T) {
		t.Parallel()

		var (
			mailer         email.MockMailer
			roleRepository roles.MockRolesRepository
			renderer       template.MockRenderer
			translatorMock translator.TranslatorMock

			command = SendRegistrationEmail{
				Identity:     "test@mail.com",
				LanguageCode: languageCode,
			}
		)

		renderer.On("Render", mock.Anything, localizedTemplate, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		translatorMock.On("Translate", registrationEmailSubject, mock.Anything).Once().Return(subject)
		defer translatorMock.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, command.Identity, subject, mock.AnythingOfType("[]uint8")).Once().Return(nil)
		defer mailer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		authTokenGenerator := auth.NewTokenGenerator(j, &roleRepository)

		err = NewSendRegisterationEmailHandler(authTokenGenerator, &mailer, mailFrom, &renderer, &translatorMock).Handle(payload)

		assert.NoError(t, err)
	})

	t.Run("error on rendering template", func(t *testing.T) {
		t.Parallel()

		var (
			mailer         email.MockMailer
			roleRepository roles.MockRolesRepository
			renderer       template.MockRenderer
			translatorMock translator.TranslatorMock

			command = SendRegistrationEmail{
				Identity:     "test@mail.com",
				LanguageCode: languageCode,
			}

			expectedError = errors.New("some error")
		)

		renderer.On("Render", mock.Anything, localizedTemplate, mock.Anything).Once().Return(expectedError)
		defer renderer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		authTokenGenerator := auth.NewTokenGenerator(j, &roleRepository)

		err = NewSendRegisterationEmailHandler(authTokenGenerator, &mailer, mailFrom, &renderer, &translatorMock).Handle(payload)

		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, err, expectedError)
	})

	t.Run("sending mail fails", func(t *testing.T) {
		t.Parallel()

		var (
			mailer         email.MockMailer
			roleRepository roles.MockRolesRepository
			renderer       template.MockRenderer
			translatorMock translator.TranslatorMock

			command = SendRegistrationEmail{
				Identity:     "test@mail.com",
				LanguageCode: languageCode,
			}

			expectedError = errors.New("some error")
		)

		renderer.On("Render", mock.Anything, localizedTemplate, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		translatorMock.On("Translate", registrationEmailSubject, mock.Anything).Once().Return(subject)
		defer translatorMock.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, command.Identity, subject, mock.AnythingOfType("[]uint8")).Once().Return(expectedError)
		defer mailer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		authTokenGenerator := auth.NewTokenGenerator(j, &roleRepository)

		err = NewSendRegisterationEmailHandler(authTokenGenerator, &mailer, mailFrom, &renderer, &translatorMock).Handle(payload)

		assert.ErrorIs(t, err, expectedError)
	})
}
