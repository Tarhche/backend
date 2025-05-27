package refresh

import (
	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository     user.Repository
	jwt                *jwt.JWT
	authTokenGenerator *auth.AuthTokenGenerator
	translator         translator.Translator
	validator          domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	jwt *jwt.JWT,
	authTokenGenerator *auth.AuthTokenGenerator,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository:     userRepository,
		jwt:                jwt,
		authTokenGenerator: authTokenGenerator,
		translator:         translator,
		validator:          validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	claims, err := uc.jwt.Verify(request.Token)
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
				"identity": uc.translator.Translate("identity_not_exists"),
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	accessToken, err := uc.authTokenGenerator.GenerateAccessToken(u.UUID)
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.authTokenGenerator.GenerateRefreshToken(u.UUID)
	if err != nil {
		return nil, err
	}

	return &Response{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
