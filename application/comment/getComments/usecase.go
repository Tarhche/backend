package getComments

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 10

type UseCase struct {
	commentRepository comment.Repository
	userRepository    user.Repository
	validator         domain.Validator
}

func NewUseCase(
	commentRepository comment.Repository,
	userRepository user.Repository,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		commentRepository: commentRepository,
		userRepository:    userRepository,
		validator:         validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	totalComments, err := uc.commentRepository.CountApprovedByObjectUUID(request.ObjectType, request.ObjectUUID)
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

	c, err := uc.commentRepository.GetApprovedByObjectUUID(
		request.ObjectType,
		request.ObjectUUID,
		offset, limit,
	)
	if err != nil {
		return nil, err
	}

	userUUIDs := make([]string, len(c))
	for i := range c {
		userUUIDs[i] = c[i].Author.UUID
	}

	u, err := uc.userRepository.GetByUUIDs(userUUIDs)
	if err != nil {
		return nil, err
	}

	for i := range c {
		for j := range u {
			if c[i].Author.UUID != u[j].UUID {
				continue
			}

			c[i].Author.Name = u[j].Name
			c[i].Author.Avatar = u[j].Avatar
		}
	}

	return NewResponse(c, totalPages, currentPage), nil
}
