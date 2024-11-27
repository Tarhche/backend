package updateuser

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository user.Repository
	validator      domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		validator:      validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	// make sure email is unique
	exists, err := uc.anotherUserExists(request.Email, request.UserUUID)
	if err != nil {
		return nil, err
	} else if exists {
		return &Response{
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
		return &Response{
			ValidationErrors: map[string]string{
				"username": "another user with this email already exists",
			},
		}, nil
	}

	u, err := uc.userRepository.GetOne(request.UserUUID)
	if err != nil {
		return nil, err
	}

	u.Name = request.Name
	u.Avatar = request.Avatar
	u.Email = request.Email
	u.Username = request.Username

	_, err = uc.userRepository.Save(&u)

	return nil, err
}

func (uc *UseCase) anotherUserExists(identity string, userUUID string) (bool, error) {
	u, err := uc.userRepository.GetOneByIdentity(identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.UUID != userUUID, nil
}
