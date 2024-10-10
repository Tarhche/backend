package register

import (
	"bytes"
	"encoding/base64"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

const (
	templateName    = "resources/view/mail/auth/register"
	registrationURL = "https://reactjs.tarhche.com/auth/verify?token="
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

	if exists, err := uc.userExists(request.Identity); err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: map[string]string{
				"identity": "user with given email already exists",
			},
		}, nil
	}

	registrationToken, err := uc.registrationToken(request.Identity)
	if err != nil {
		return nil, err
	}

	registrationToken = base64.URLEncoding.EncodeToString([]byte(registrationToken))

	var msg bytes.Buffer
	viewData := map[string]string{"registrationURL": registrationURL + registrationToken}
	if err := uc.template.Render(&msg, templateName, viewData); err != nil {
		return nil, err
	}

	if err := uc.mailer.SendMail(uc.mailFrom, request.Identity, "Registration", msg.Bytes()); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

func (uc *UseCase) userExists(identity string) (bool, error) {
	u, err := uc.userRepository.GetOneByIdentity(identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.Email == identity || u.Username == identity, nil
}

func (uc *UseCase) registrationToken(identity string) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(identity)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(24 * time.Hour))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{auth.RegistrationToken})

	return uc.jwt.Generate(b.Build())
}
