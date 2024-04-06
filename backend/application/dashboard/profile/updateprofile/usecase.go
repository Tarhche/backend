package updateprofile

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository user.Repository
}

func NewUseCase(userRepository user.Repository) *UseCase {
	return &UseCase{
		userRepository: userRepository,
	}
}

func (uc *UseCase) UpdateProfile(request Request) (*UpdateProfileResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &UpdateProfileResponse{
			ValidationErrors: validation,
		}, nil
	}

	// make sure email is unique
	exists, err := uc.anotherUserExists(request.Email, request.UserUUID)
	if err != nil {
		return nil, err
	} else if exists {
		return &UpdateProfileResponse{
			ValidationErrors: map[string]string{
				"email": "another user with this email already exists",
			},
		}, nil
	}

	// make sure username is unique
	exists, err = uc.anotherUserExists(request.Username, request.UserUUID)
	if err != nil {
		return nil, err
	} else if exists {
		return &UpdateProfileResponse{
			ValidationErrors: map[string]string{
				"username": "another user with this email already exists",
			},
		}, nil
	}

	user, err := uc.userRepository.GetOne(request.UserUUID)
	if err != nil {
		return nil, err
	}

	user.Name = request.Name
	user.Avatar = request.Avatar
	user.Email = request.Email
	user.Username = request.Username

	err = uc.userRepository.Save(&user)
	if err != nil {
		return nil, err
	}

	return &UpdateProfileResponse{}, err
}

func (uc *UseCase) anotherUserExists(identity string, currentUserUUID string) (bool, error) {
	u, err := uc.userRepository.GetOneByIdentity(identity)
	if err == domain.ErrNotExists {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.UUID != currentUserUUID, nil
}
