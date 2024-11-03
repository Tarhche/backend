package register

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/template"
)

func TestHandler_Execute(t *testing.T) {
	privateKey, err := ecdsa.Generate()
	assert.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	mailFrom := "info@noreply.nowhere.loc"

	t.Run("sends registration mail", func(t *testing.T) {
		var (
			mailer   email.MockMailer
			renderer template.MockRenderer

			command = SendRegistrationEmail{
				Identity: "test@mail.com",
			}
		)

		renderer.On("Render", mock.Anything, templateName, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, command.Identity, "Registration", mock.AnythingOfType("[]uint8")).Once().Return(nil)
		defer mailer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		err = NewSendRegisterationEmailHandler(j, &mailer, mailFrom, &renderer).Handle(payload)

		assert.NoError(t, err)
	})

	t.Run("error on rendering template", func(t *testing.T) {
		var (
			mailer   email.MockMailer
			renderer template.MockRenderer

			command = SendRegistrationEmail{
				Identity: "test@mail.com",
			}

			expectedError = errors.New("some error")
		)

		renderer.On("Render", mock.Anything, templateName, mock.Anything).Once().Return(expectedError)
		defer renderer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		err = NewSendRegisterationEmailHandler(j, &mailer, mailFrom, &renderer).Handle(payload)

		mailer.AssertNotCalled(t, "SendMail")

		assert.ErrorIs(t, err, expectedError)
	})

	t.Run("sending mail fails", func(t *testing.T) {
		var (
			mailer   email.MockMailer
			renderer template.MockRenderer

			command = SendRegistrationEmail{
				Identity: "test@mail.com",
			}

			expectedError = errors.New("some error")
		)

		renderer.On("Render", mock.Anything, templateName, mock.Anything).Once().Return(nil)
		defer renderer.AssertExpectations(t)

		mailer.On("SendMail", mailFrom, command.Identity, "Registration", mock.AnythingOfType("[]uint8")).Once().Return(expectedError)
		defer mailer.AssertExpectations(t)

		payload, err := json.Marshal(command)
		assert.NoError(t, err)

		err = NewSendRegisterationEmailHandler(j, &mailer, mailFrom, &renderer).Handle(payload)

		assert.ErrorIs(t, err, expectedError)
	})
}
