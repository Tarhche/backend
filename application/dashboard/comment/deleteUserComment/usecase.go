package deleteUserComment

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

func (uc *UseCase) Execute(request *Request) error {
	return uc.commentRepository.DeleteByAuthorUUID(request.CommentUUID, request.UserUUID)
}
