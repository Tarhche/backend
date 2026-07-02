package getUserComments

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 10

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

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	totalComments, err := uc.commentRepository.CountByAuthorUUID(ctx, request.UserUUID)
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	if currentPage == 0 {
		currentPage = 1
	}

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalComments / limit

	if (totalPages * limit) != totalComments {
		totalPages++
	}

	c, err := uc.commentRepository.GetAllByAuthorUUID(ctx, request.UserUUID, offset, limit)
	if err != nil {
		return nil, err
	}

	u, err := uc.userRepository.GetOne(ctx, request.UserUUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(c, u, totalPages, currentPage), nil
}
