package deleteUserBookmark

import (
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

func (uc *UseCase) Execute(request *Request) error {
	return uc.bookmarkRepository.DeleteByOwnerUUID(
		request.OwnerUUID,
		request.ObjectType,
		request.ObjectUUID,
	)
}
