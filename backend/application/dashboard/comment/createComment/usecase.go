package createComment

import (
	"github.com/khanzadimahdi/testproject/domain/author"
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

func (uc *UseCase) CreateComment(request Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
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
		ApprovedAt: request.ApprovedAt,
	}

	_, err := uc.commentRepository.Save(&c)
	if err != nil {
		return nil, err
	}

	return &Response{}, err
}
