package getUserBookmarks

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

const limit = 10

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

	totalArticles, err := uc.bookmarkRepository.CountByOwnerUUID(request.OwnerUUID)
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

	totalPages := totalArticles / limit

	if (totalPages * limit) != totalArticles {
		totalPages++
	}

	b, err := uc.bookmarkRepository.GetAllByOwnerUUID(request.OwnerUUID, offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(b, totalPages, currentPage), nil
}
