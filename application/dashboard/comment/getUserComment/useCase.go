package getUserComment

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	commentRepository comment.Repository
	userRepository    user.Repository
}

func NewUseCase(commentRepository comment.Repository, userRepository user.Repository) *UseCase {
	return &UseCase{
		commentRepository: commentRepository,
		userRepository:    userRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, UUID string, userUUID string) (*Response, error) {
	c, err := uc.commentRepository.GetOneByAuthorUUID(ctx, UUID, userUUID)
	if err != nil {
		return nil, err
	}

	u, err := uc.userRepository.GetOne(ctx, c.AuthorUUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(c, u), nil
}
