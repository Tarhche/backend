package getComment

import (
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

func (uc *UseCase) Execute(UUID string) (*Response, error) {
	c, err := uc.commentRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	u, err := uc.userRepository.GetOne(c.AuthorUUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(c, u), nil
}
