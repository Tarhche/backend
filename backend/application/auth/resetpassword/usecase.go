package resetpassword

import (
	"crypto/rand"
	"encoding/base64"
	"errors"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository user.Repository
	hasher         password.Hasher
	jwt            *jwt.JWT
	translator     translator.Translator
	validator      domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	hasher password.Hasher, JWT *jwt.JWT,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		hasher:         hasher,
		jwt:            JWT,
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

	resetPasswordToken, err := base64.URLEncoding.DecodeString(request.Token)
	if err != nil {
		return nil, err
	}

	claims, err := uc.jwt.Verify(string(resetPasswordToken))
	if err != nil {
		return nil, err
	}

	audiences, err := claims.GetAudience()
	if err != nil || len(audiences) == 0 || audiences[0] != auth.ResetPasswordToken {
		return nil, errors.New("token is not valid")
	}

	userUUID, err := claims.GetSubject()
	if err != nil || len(userUUID) == 0 {
		return nil, errors.New("token is not valid")
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

	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	u.PasswordHash = password.Hash{
		Value: uc.hasher.Hash([]byte(request.Password), salt),
		Salt:  salt,
	}

	if _, err := uc.userRepository.Save(&u); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
