package createComment

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
)

type UseCase struct {
	commentRepository comment.Repository
	validator         domain.Validator
}

func NewUseCase(
	commentRepository comment.Repository,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		commentRepository: commentRepository,
		validator:         validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	c := comment.Comment{
		Body: request.Body,
		Author: author.Author{
			UUID: request.AuthorUUID,
		},
		ParentUUID: request.ParentUUID,
		ObjectUUID: request.ObjectUUID,
		ObjectType: request.ObjectType,
	}

	_, err := uc.commentRepository.Save(&c)

	return nil, err
}
