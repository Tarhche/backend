package login

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository user.Repository
	JWT            *jwt.JWT
	Hasher         password.Hasher
	translator     translator.Translator
	validator      domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	JWT *jwt.JWT, hasher password.Hasher,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		JWT:            JWT,
		Hasher:         hasher,
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

func (uc *UseCase) passwordIsValid(u user.User, password []byte) bool {
	return uc.Hasher.Equal(password, u.PasswordHash.Value, u.PasswordHash.Salt)
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
