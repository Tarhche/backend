package getUserComments

import (
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

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	totalComments, err := uc.commentRepository.CountByAuthorUUID(request.UserUUID)
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

	c, err := uc.commentRepository.GetAllByAuthorUUID(request.UserUUID, offset, limit)
	if err != nil {
		return nil, err
	}

	u, err := uc.userRepository.GetOne(request.UserUUID)
	if err != nil {
		return nil, err
	}

	for i := range c {
		c[i].Author.Name = u.Name
		c[i].Author.Avatar = u.Avatar
	}

	return NewResponse(c, totalPages, currentPage), nil
}
