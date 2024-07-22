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
	totalComments, err := uc.commentRepository.CountApprovedByObjectUUID(request.ObjectType, request.ObjectUUID)
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

	c, err := uc.commentRepository.GetApprovedByObjectUUID(
		request.ObjectType,
		request.ObjectUUID,
		offset, limit,
	)
	if err != nil {
		return nil, err
	}

	return NewResponse(c, totalPages, currentPage), nil
}
