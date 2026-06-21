package register

import (
	"bytes"
	"encoding/base64"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	translatorcontract "github.com/khanzadimahdi/testproject/domain/translator"
)

const (
	SendRegisterationEmailName = "sendRegistrationEmail"

	templateName             = "mail/auth/register"
	registrationEmailSubject = "registration_email_subject"
	registrationURL          = "https://tarhche.com/auth/verify?token="
)

// SendRegistrationEmail command
type SendRegistrationEmail struct {
	Identity     string `json:"identity"`
	LanguageCode string `json:"language_code"`
}

// SendRegisterationEmailHandler handles SendMail command
type sendRegisterationEmailHandler struct {
	authTokenGenerator *auth.AuthTokenGenerator
	mailer             domain.Mailer
	mailFrom           string
	template           domain.Renderer
	translator         translatorcontract.Translator
}

var _ domain.MessageHandler = &sendRegisterationEmailHandler{}

func NewSendRegisterationEmailHandler(
	authTokenGenerator *auth.AuthTokenGenerator,
	mailer domain.Mailer,
	mailFrom string,
	template domain.Renderer,
	translator translatorcontract.Translator,
) *sendRegisterationEmailHandler {
	return &sendRegisterationEmailHandler{
		authTokenGenerator: authTokenGenerator,
		mailer:             mailer,
		mailFrom:           mailFrom,
		template:           template,
		translator:         translator,
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
	if err := h.template.Render(&msg, templateName+"."+command.LanguageCode, viewData); err != nil {
		return err
	}

	subject := h.translator.Translate(registrationEmailSubject, translatorcontract.WithLocale(command.LanguageCode))

	return h.mailer.SendMail(h.mailFrom, command.Identity, subject, msg.Bytes())
}
