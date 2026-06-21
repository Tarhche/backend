package forgetpassword

import (
	"bytes"
	"encoding/base64"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	translatorcontract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const (
	SendForgetPasswordEmailName = "sendForgetPasswordEmail"

	resetPasswordEmailSubject = "reset_password_email_subject"
)

// SendForgetPasswordEmail command
type SendForgetPasswordEmail struct {
	Identity string `json:"identity"`
}

// SendForgetPasswordEmailHandler handles SendMail command
type sendForgetPasswordEmailHandler struct {
	userRepository     user.Repository
	authTokenGenerator *auth.AuthTokenGenerator
	mailer             domain.Mailer
	mailFrom           string
	template           domain.Renderer
	translator         translatorcontract.Translator
}

var _ domain.MessageHandler = &sendForgetPasswordEmailHandler{}

func NewSendForgetPasswordEmailHandler(
	userRepository user.Repository,
	authTokenGenerator *auth.AuthTokenGenerator,
	mailer domain.Mailer,
	mailFrom string,
	template domain.Renderer,
	translator translatorcontract.Translator,
) *sendForgetPasswordEmailHandler {
	return &sendForgetPasswordEmailHandler{
		userRepository:     userRepository,
		authTokenGenerator: authTokenGenerator,
		mailer:             mailer,
		mailFrom:           mailFrom,
		template:           template,
		translator:         translator,
	}
}

func (h *sendForgetPasswordEmailHandler) Handle(data []byte) error {
	var command SendForgetPasswordEmail
	if err := json.Unmarshal(data, &command); err != nil {
		return err
	}

	u, err := h.userRepository.GetOneByIdentity(command.Identity)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	resetPasswordToken, err := h.authTokenGenerator.GenerateResetPasswordToken(u.UUID)
	if err != nil {
		return err
	}

	resetPasswordToken = base64.URLEncoding.EncodeToString([]byte(resetPasswordToken))

	var msg bytes.Buffer
	viewData := map[string]string{"resetPasswordURL": resetPasswordURL + resetPasswordToken}
	if err := h.template.Render(&msg, templateName+"."+u.LanguageCode, viewData); err != nil {
		return err
	}

	subject := h.translator.Translate(resetPasswordEmailSubject, translatorcontract.WithLocale(u.LanguageCode))

	return h.mailer.SendMail(h.mailFrom, u.Email, subject, msg.Bytes())
}
