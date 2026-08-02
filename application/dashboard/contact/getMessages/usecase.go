package getMessages

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/contact"
)

const limit = 10

type UseCase struct {
	contactRepository contact.Repository
}

func NewUseCase(contactRepository contact.Repository) *UseCase {
	return &UseCase{
		contactRepository: contactRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	totalMessages, err := uc.contactRepository.Count(ctx)
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

	totalPages := totalMessages / limit

	if (totalPages * limit) != totalMessages {
		totalPages++
	}

	m, err := uc.contactRepository.GetAll(ctx, offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(m, totalPages, currentPage), nil
}
