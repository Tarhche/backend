package deleteuser

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

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.userRepository.Delete(ctx, request.UserUUID)
}
