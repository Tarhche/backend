package bookmarkExists

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
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
