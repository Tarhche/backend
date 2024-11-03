package register

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

const (
	SendRegisterationEmailName = "sendRegistrationEmail"

	templateName    = "resources/view/mail/auth/register"
	registrationURL = "https://tarhche.com/auth/verify?token="
)

// SendRegistrationEmail command
type SendRegistrationEmail struct {
	Identity string `json:"identity"`
}

// SendRegisterationEmailHandler handles SendMail command
type sendRegisterationEmailHandler struct {
	jwt      *jwt.JWT
	mailer   domain.Mailer
	mailFrom string
	template domain.Renderer
}

var _ domain.MessageHandler = &sendRegisterationEmailHandler{}

func NewSendRegisterationEmailHandler(
	JWT *jwt.JWT,
	mailer domain.Mailer,
	mailFrom string,
	template domain.Renderer,
) *sendRegisterationEmailHandler {
	return &sendRegisterationEmailHandler{
		jwt:      JWT,
		mailer:   mailer,
		mailFrom: mailFrom,
		template: template,
	}
}

func (h *sendRegisterationEmailHandler) Handle(data []byte) error {
	var command SendRegistrationEmail
	if err := json.Unmarshal(data, &command); err != nil {
		return err
	}

	registrationToken, err := h.registrationToken(command.Identity)
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

func (h *sendRegisterationEmailHandler) registrationToken(identity string) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(identity)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(24 * time.Hour))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{auth.RegistrationToken})

	return h.jwt.Generate(b.Build())
}
