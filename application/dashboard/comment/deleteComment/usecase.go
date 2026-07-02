package deleteComment

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/comment"
)

type UseCase struct {
	commentRepository comment.Repository
}

func NewUseCase(commentRepository comment.Repository) *UseCase {
	return &UseCase{
		commentRepository: commentRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.commentRepository.Delete(ctx, request.CommentUUID)
}
