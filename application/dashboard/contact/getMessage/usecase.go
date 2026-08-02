package getMessage

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/contact"
)

type UseCase struct {
	contactRepository contact.Repository
}

func NewUseCase(contactRepository contact.Repository) *UseCase {
	return &UseCase{
		contactRepository: contactRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, UUID string) (*Response, error) {
	m, err := uc.contactRepository.GetOne(ctx, UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(m), nil
}
