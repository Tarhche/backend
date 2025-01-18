package updateUserComment

import (
	"github.com/khanzadimahdi/testproject/domain"
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

	c, err := uc.commentRepository.GetOneByAuthorUUID(request.UUID, request.UserUUID)
	if err != nil {
		return nil, err
	}

	if !c.ApprovedAt.IsZero() {
		return nil, comment.ErrUpdatingAnApprovedCommentNotAllowed
	}

	c.Body = request.Body

	_, err = uc.commentRepository.Save(&c)

	return nil, err
}
