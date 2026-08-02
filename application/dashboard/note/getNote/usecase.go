package getnote

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	noteRepository note.Repository
	userRepository user.Repository
}

func NewUseCase(noteRepository note.Repository, userRepository user.Repository) *UseCase {
	return &UseCase{
		noteRepository: noteRepository,
		userRepository: userRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	n, err := uc.noteRepository.GetByCorrelationUUIDAndLanguage(ctx, request.CorrelationUUID, request.LanguageCode)
	if err != nil {
		return nil, err
	}

	// Scoped to the caller's own notes: someone else's note is treated as if it
	// doesn't exist.
	if len(request.OwnerUUID) > 0 && n.AuthorUUID != request.OwnerUUID {
		return nil, domain.ErrNotExists
	}

	u, err := uc.userRepository.GetOne(ctx, n.AuthorUUID)
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	return NewResponse(n, u), nil
}
