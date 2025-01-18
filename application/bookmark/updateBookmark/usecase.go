package updateBookmark

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

	if !request.Keep {
		if err := uc.bookmarkRepository.DeleteByOwnerUUID(
			request.OwnerUUID,
			request.ObjectType,
			request.ObjectUUID,
		); err != nil {
			return nil, err
		}

		return nil, nil
	}

	b := bookmark.Bookmark{
		Title:      request.Title,
		ObjectUUID: request.ObjectUUID,
		ObjectType: request.ObjectType,
		OwnerUUID:  request.OwnerUUID,
	}

	if _, err := uc.bookmarkRepository.Save(&b); err != nil {
		return nil, err
	}

	return nil, nil
}
