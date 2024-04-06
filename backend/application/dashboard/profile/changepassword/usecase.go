package changepassword

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

func (uc *UseCase) ChangePassword(request Request) (*ChangePasswordResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &ChangePasswordResponse{
			ValidationErrors: validation,
		}, nil
	}

	u, err := uc.userRepository.GetOne(request.UserUUID)
	if err != nil {
		return nil, err
	}

	if !uc.passwordIsValid(u, []byte(request.CurrentPassword)) {
		return &ChangePasswordResponse{
			ValidationErrors: validationErrors{
				"current_password": "current password is not valid",
			},
		}, nil
	}

	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	u.PasswordHash = password.Hash{
		Value: uc.hasher.Hash([]byte(request.NewPassword), salt),
		Salt:  salt,
	}

	if err := uc.userRepository.Save(&u); err != nil {
		return nil, err
	}

	return &ChangePasswordResponse{}, err
}

func (uc *UseCase) passwordIsValid(u user.User, password []byte) bool {
	return uc.hasher.Equal(password, u.PasswordHash.Value, u.PasswordHash.Salt)
}
