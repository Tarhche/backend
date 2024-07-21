package getComments

import (
	"github.com/khanzadimahdi/testproject/domain/comment"
)

const limit = 10

type UseCase struct {
	commentRepository comment.Repository
}

func NewUseCase(commentRepository comment.Repository) *UseCase {
	return &UseCase{
		commentRepository: commentRepository,
	}
}

func (uc *UseCase) GetComments(request *Request) (*Response, error) {
	totalComments, err := uc.commentRepository.Count()
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalComments / limit

	if (totalPages * limit) != totalComments {
		totalPages++
	}

	c, err := uc.commentRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(c, totalPages, currentPage), nil
}
