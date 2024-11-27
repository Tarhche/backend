package deleteUserBookmark

import (
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

	err := uc.bookmarkRepository.DeleteByOwnerUUID(
		request.OwnerUUID,
		request.ObjectType,
		request.ObjectUUID,
	)

	return nil, err
}
