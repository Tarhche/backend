package createuser

import (
	"crypto/rand"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository user.Repository
	hasher         password.Hasher
	validator      domain.Validator
	translator     translator.Translator
}

func NewUseCase(
	userRepository user.Repository,
	hasher password.Hasher,
	validator domain.Validator,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		hasher:         hasher,
		validator:      validator,
		translator:     translator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if ok, err := uc.anotherUserExists(request.Email); err != nil {
		return nil, err
	} else if ok {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"email": uc.translator.Translate("another user with same email already exists"),
			},
		}, nil
	}

	if ok, err := uc.anotherUserExists(request.Username); err != nil {
		return nil, err
	} else if ok {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"username": uc.translator.Translate("another user with same username already exists"),
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
	u, err := uc.userRepository.GetOneByIdentity(identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.Email == identity || u.Username == identity, nil
}

func (uc *UseCase) createUser(request *Request) (string, error) {
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
