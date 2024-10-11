package forgetpassword

import (
	"bytes"
	"encoding/base64"
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

const (
	templateName     = "resources/view/mail/auth/reset-password"
	resetPasswordURL = "https://tarhche.com/auth/reset-password?token="
)

type UseCase struct {
	userRepository user.Repository
	jwt            *jwt.JWT
	mailer         domain.Mailer
	mailFrom       string
	template       domain.Renderer
}

func NewUseCase(
	userRepository user.Repository,
	JWT *jwt.JWT,
	mailer domain.Mailer,
	mailFrom string,
	template domain.Renderer,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		jwt:            JWT,
		mailer:         mailer,
		mailFrom:       mailFrom,
		template:       template,
	}
}

func (uc *UseCase) Execute(request Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	u, err := uc.userRepository.GetOneByIdentity(request.Identity)
	if err == domain.ErrNotExists {
		return &Response{
			ValidationErrors: validationErrors{
				"identity": "identity (email/username) not exists",
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	resetPasswordToken, err := uc.resetPasswordToken(u)
	if err != nil {
		return nil, err
	}

	resetPasswordToken = base64.URLEncoding.EncodeToString([]byte(resetPasswordToken))

	var msg bytes.Buffer
	viewData := map[string]string{"resetPasswordURL": resetPasswordURL + resetPasswordToken}
	if err := uc.template.Render(&msg, templateName, viewData); err != nil {
		return nil, err
	}

	if err := uc.mailer.SendMail(uc.mailFrom, u.Email, "Reset Password", msg.Bytes()); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

func (uc *UseCase) resetPasswordToken(u user.User) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(10 * time.Minute))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{auth.ResetPasswordToken})

	return uc.jwt.Generate(b.Build())
}
