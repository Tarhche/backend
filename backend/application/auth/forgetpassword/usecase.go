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

type UseCase struct {
	userRepository user.Repository
	jwt            *jwt.JWT
	mailer         domain.Mailer
	mailFrom       string
}

func NewUseCase(
	userRepository user.Repository,
	JWT *jwt.JWT,
	mailer domain.Mailer,
	mailFrom string,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		jwt:            JWT,
		mailer:         mailer,
		mailFrom:       mailFrom,
	}
}

func (uc *UseCase) SendResetToken(request Request) (*ForgetResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &ForgetResponse{
			ValidationErrors: validation,
		}, nil
	}

	u, err := uc.userRepository.GetOneByIdentity(request.Identity)
	if err != nil {
		return nil, err
	}

	resetPasswordToken, err := uc.resetPasswordToken(u)
	if err != nil {
		return nil, err
	}

	resetPasswordToken = base64.URLEncoding.EncodeToString([]byte(resetPasswordToken))

	var msg bytes.Buffer
	if _, err := msg.WriteString(resetPasswordToken); err != nil {
		return nil, err
	}

	if err := uc.mailer.SendMail(uc.mailFrom, u.Email, "Reset Password", msg.Bytes()); err != nil {
		return nil, err
	}

	return &ForgetResponse{}, nil
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
