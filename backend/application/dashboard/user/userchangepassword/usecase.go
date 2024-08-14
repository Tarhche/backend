package userchangepassword

import (
	"crypto/rand"

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

func (uc *UseCase) Execute(request Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
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

	if _, err := uc.userRepository.Save(&u); err != nil {
		return nil, err
	}

	return &Response{}, err
}
