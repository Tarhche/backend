package deleteMessage

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

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.contactRepository.Delete(ctx, request.MessageUUID)
}
