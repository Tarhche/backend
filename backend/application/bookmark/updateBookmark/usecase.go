package updateBookmark

import (
	"log"

	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

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
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	log.Println("keep", !request.Keep)

	if !request.Keep {
		if err := uc.bookmarkRepository.DeleteByOwnerUUID(
			request.OwnerUUID,
			request.ObjectType,
			request.ObjectUUID,
		); err != nil {
			return nil, err
		}

		return &Response{}, nil
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

	return &Response{}, nil
}
