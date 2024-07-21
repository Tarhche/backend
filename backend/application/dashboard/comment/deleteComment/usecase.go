package deleteComment

import (
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

func (uc *UseCase) DeleteComment(request Request) error {
	return uc.commentRepository.Delete(request.CommentUUID)
}
