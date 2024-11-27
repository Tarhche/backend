package userchangepassword

import (
	"crypto/rand"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository user.Repository
	hasher         password.Hasher
	validator      domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	hasher password.Hasher,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		hasher:         hasher,
		validator:      validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	u, err := uc.userRepository.GetOne(request.UserUUID)
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	u.PasswordHash = password.Hash{
		Value: uc.hasher.Hash([]byte(request.NewPassword), salt),
		Salt:  salt,
	}

	_, err = uc.userRepository.Save(&u)

	return nil, err
}
