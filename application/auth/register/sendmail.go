package register

import (
	"bytes"
	"encoding/base64"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
)

const (
	SendRegisterationEmailName = "sendRegistrationEmail"

	templateName    = "mail/auth/register"
	registrationURL = "https://tarhche.com/auth/verify?token="
)

// SendRegistrationEmail command
type SendRegistrationEmail struct {
	Identity string `json:"identity"`
}

// SendRegisterationEmailHandler handles SendMail command
type sendRegisterationEmailHandler struct {
	authTokenGenerator *auth.AuthTokenGenerator
	mailer             domain.Mailer
	mailFrom           string
	template           domain.Renderer
}

var _ domain.MessageHandler = &sendRegisterationEmailHandler{}

func NewSendRegisterationEmailHandler(
	authTokenGenerator *auth.AuthTokenGenerator,
	mailer domain.Mailer,
	mailFrom string,
	template domain.Renderer,
) *sendRegisterationEmailHandler {
	return &sendRegisterationEmailHandler{
		authTokenGenerator: authTokenGenerator,
		mailer:             mailer,
		mailFrom:           mailFrom,
		template:           template,
	}
}

func (h *sendRegisterationEmailHandler) Handle(data []byte) error {
	var command SendRegistrationEmail
	if err := json.Unmarshal(data, &command); err != nil {
		return err
	}

	registrationToken, err := h.authTokenGenerator.GenerateRegistrationToken(command.Identity)
	if err != nil {
		return err
	}

	registrationToken = base64.URLEncoding.EncodeToString([]byte(registrationToken))

	var msg bytes.Buffer
	viewData := map[string]string{"registrationURL": registrationURL + registrationToken}
	if err := h.template.Render(&msg, templateName, viewData); err != nil {
		return err
	}

	return h.mailer.SendMail(h.mailFrom, command.Identity, "Registration", msg.Bytes())
}
