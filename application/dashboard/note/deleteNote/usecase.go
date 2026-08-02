package deletenote

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/note"
)

type UseCase struct {
	noteRepository note.Repository
}

func NewUseCase(noteRepository note.Repository) *UseCase {
	return &UseCase{
		noteRepository: noteRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	if len(request.OwnerUUID) > 0 {
		existing, err := uc.noteRepository.GetByCorrelationUUIDAndLanguage(ctx, request.CorrelationUUID, request.LanguageCode)
		if err != nil {
			return err
		}

		// Scoped to the caller's own notes: someone else's note is treated as if
		// it doesn't exist.
		if existing.AuthorUUID != request.OwnerUUID {
			return domain.ErrNotExists
		}
	}

	return uc.noteRepository.DeleteByCorrelationUUIDAndLanguage(ctx, request.CorrelationUUID, request.LanguageCode)
}
