package getComment

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

func (uc *UseCase) GetArticle(UUID string) (*Response, error) {
	c, err := uc.commentRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(c), nil
}
