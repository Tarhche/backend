package createuser

import (
	"crypto/rand"
	"errors"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository user.Repository
	hasher         password.Hasher
}

func NewUseCase(userRepository user.Repository, hasher password.Hasher) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		hasher:         hasher,
	}
}

func (uc *UseCase) CreateUser(request Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	if ok, err := uc.anotherUserExists(request.Email); err != nil {
		return nil, err
	} else if !ok {
		return &Response{
			ValidationErrors: validationErrors{
				"email": "another user with this email already exists",
			},
		}, nil
	}

	if ok, err := uc.anotherUserExists(request.Username); err != nil {
		return nil, err
	} else if !ok {
		return &Response{
			ValidationErrors: validationErrors{
				"username": "another user with this username already exists",
			},
		}, nil
	}

	UUID, err := uc.createUser(request)
	if err != nil {
		return nil, err
	}

	return &Response{UUID: UUID}, err
}

func (uc *UseCase) anotherUserExists(identity string) (bool, error) {
	_, err := uc.userRepository.GetOneByIdentity(identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func (uc *UseCase) createUser(request Request) (string, error) {
	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	u := user.User{
		Name:     request.Name,
		Avatar:   request.Avatar,
		Email:    request.Email,
		Username: request.Username,
		PasswordHash: password.Hash{
			Value: uc.hasher.Hash([]byte(request.Password), salt),
			Salt:  salt,
		},
	}

	return uc.userRepository.Save(&u)
}
