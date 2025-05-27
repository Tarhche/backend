package login

import (
	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository     user.Repository
	authTokenGenerator *auth.AuthTokenGenerator
	Hasher             password.Hasher
	translator         translator.Translator
	validator          domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	authTokenGenerator *auth.AuthTokenGenerator,
	hasher password.Hasher,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository:     userRepository,
		authTokenGenerator: authTokenGenerator,
		Hasher:             hasher,
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

	u, err := uc.userRepository.GetOneByIdentity(request.Identity)
	if err == domain.ErrNotExists {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"identity": uc.translator.Translate("invalid_identity_or_password"),
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	if !uc.passwordIsValid(u, []byte(request.Password)) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"identity": uc.translator.Translate("invalid_identity_or_password"),
			},
		}, nil
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

func (uc *UseCase) passwordIsValid(u user.User, password []byte) bool {
	return uc.Hasher.Equal(password, u.PasswordHash.Value, u.PasswordHash.Salt)
}
