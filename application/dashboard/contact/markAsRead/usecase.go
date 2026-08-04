package markAsRead

import (
	"context"
	"time"

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

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	m, err := uc.contactRepository.GetOne(ctx, request.MessageUUID)
	if err != nil {
		return nil, err
	}

	switch {
	case !request.Read:
		m.ReadAt = time.Time{}
	case !m.IsRead():
		// The read time stamps itself the first time the message is marked as
		// read; marking an already-read message keeps the original moment.
		m.ReadAt = time.Now()
	}

	if _, err := uc.contactRepository.Save(ctx, &m); err != nil {
		return nil, err
	}

	return NewResponse(m), nil
}
