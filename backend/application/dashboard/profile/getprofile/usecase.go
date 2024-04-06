package getprofile

import (
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

func (uc *UseCase) Profile(UUID string) (*GetProfileResponse, error) {
	u, err := uc.userRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return &GetProfileResponse{
		UUID:     UUID,
		Name:     u.Name,
		Avatar:   u.Avatar,
		Email:    u.Email,
		Username: u.Username,
	}, err
}
