package verify

import (
	"crypto/rand"
	"errors"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository user.Repository
	hasher         password.Hasher
	jwt            *jwt.JWT
}

func NewUseCase(userRepository user.Repository, hasher password.Hasher, JWT *jwt.JWT) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		hasher:         hasher,
		jwt:            JWT,
	}
}

func (uc *UseCase) Execute(request Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	claims, err := uc.jwt.Verify(request.Token)
	if err != nil {
		return &Response{
			ValidationErrors: validationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	if audiences, err := claims.GetAudience(); err != nil || len(audiences) == 0 || audiences[0] != auth.RegistrationToken {
		return &Response{
			ValidationErrors: validationErrors{
				"token": "registration token is not valid",
			},
		}, nil
	}

	identity, err := claims.GetSubject()
	if err != nil {
		return &Response{
			ValidationErrors: validationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	if exists, err := uc.identityExists(identity); err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: map[string]string{
				"identity": "user already exists",
			},
		}, nil
	}

	if exists, err := uc.identityExists(request.Username); err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: map[string]string{
				"username": "user with given username already exists",
			},
		}, nil
	}

	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	u := user.User{
		Name:     request.Name,
		Username: request.Username,
		Email:    identity,
		PasswordHash: password.Hash{
			Value: uc.hasher.Hash([]byte(request.Password), salt),
			Salt:  salt,
		},
	}

	if _, err := uc.userRepository.Save(&u); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

func (uc *UseCase) identityExists(identity string) (bool, error) {
	u, err := uc.userRepository.GetOneByIdentity(identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.UUID == identity || u.Email == identity || u.Username == identity, nil
}
