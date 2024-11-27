package refresh

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository user.Repository
	JWT            *jwt.JWT
	translator     translator.Translator
	validator      domain.Validator
}

func NewUseCase(
	userRepository user.Repository, JWT *jwt.JWT,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		JWT:            JWT,
		translator:     translator,
		validator:      validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	claims, err := uc.JWT.Verify(request.Token)
	if err != nil {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	if audiences, err := claims.GetAudience(); err != nil || len(audiences) == 0 || audiences[0] != auth.RefreshToken {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	userUUID, err := claims.GetSubject()
	if err != nil {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	u, err := uc.userRepository.GetOne(userUUID)
	if err == domain.ErrNotExists {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"identity": uc.translator.Translate("identity (email/username) not exists"),
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	accessToken, err := uc.generateAccessToken(u)
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.generateRefreshToken(u)
	if err != nil {
		return nil, err
	}

	return &Response{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (uc *UseCase) generateAccessToken(u user.User) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(15 * time.Minute))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{auth.AccessToken})

	return uc.JWT.Generate(b.Build())
}

func (uc *UseCase) generateRefreshToken(u user.User) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(2 * 24 * time.Hour))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{auth.RefreshToken})

	return uc.JWT.Generate(b.Build())
}
