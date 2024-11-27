package bookmarkExists

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

type UseCase struct {
	bookmarkRepository bookmark.Repository
	validator          domain.Validator
}

func NewUseCase(
	bookmarkRepository bookmark.Repository,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		bookmarkRepository: bookmarkRepository,
		validator:          validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if _, err := uc.bookmarkRepository.GetByOwnerUUID(request.OwnerUUID, request.ObjectType, request.ObjectUUID); errors.Is(err, domain.ErrNotExists) {
		return &Response{
			Exist: false,
		}, nil
	} else if err != nil {
		return nil, err
	}

	return &Response{
		Exist: true,
	}, nil
}
