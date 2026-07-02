package getuser

import (
	"context"

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

func (uc *UseCase) Execute(ctx context.Context, UUID string) (*Response, error) {
	u, err := uc.userRepository.GetOne(ctx, UUID)
	if err != nil {
		return nil, err
	}

	return &Response{
		UUID:         UUID,
		Name:         u.Name,
		Avatar:       u.Avatar,
		Email:        u.Email,
		Username:     u.Username,
		LanguageCode: u.LanguageCode,
	}, err
}
