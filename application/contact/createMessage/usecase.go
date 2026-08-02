package createMessage

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/contact"
)

type UseCase struct {
	contactRepository contact.Repository
	validator         domain.Validator
}

func NewUseCase(
	contactRepository contact.Repository,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		contactRepository: contactRepository,
		validator:         validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	m := contact.Message{
		Subject: request.Subject,
		Body:    request.Body,
		Email:   request.Email,
		Phone:   request.Phone,
	}

	_, err := uc.contactRepository.Save(ctx, &m)

	return nil, err
}
