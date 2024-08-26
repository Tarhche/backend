package getUserBookmarks

import (
	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

const limit = 10

type UseCase struct {
	bookmarkRepository bookmark.Repository
}

func NewUseCase(
	bookmarkRepository bookmark.Repository,
) *UseCase {
	return &UseCase{
		bookmarkRepository: bookmarkRepository,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
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
