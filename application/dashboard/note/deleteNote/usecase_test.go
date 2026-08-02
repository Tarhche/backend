package deletenote

import (
	"context"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/notes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deletes a note", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository notes.MockNotesRepository

			request = Request{CorrelationUUID: "correlation-1", LanguageCode: "EN"}
		)

		notesRepository.On("DeleteByCorrelationUUIDAndLanguage", mock.Anything, "correlation-1", "EN").Once().Return(nil)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository)
		err := usecase.Execute(context.Background(), &request)

		notesRepository.AssertNotCalled(t, "GetByCorrelationUUIDAndLanguage")

		assert.NoError(t, err)
	})

	t.Run("deletes the owner's own note", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository notes.MockNotesRepository

			existing = note.Note{UUID: "note-uuid-1", AuthorUUID: "author-uuid-1"}
			request  = Request{CorrelationUUID: "correlation-1", LanguageCode: "EN", OwnerUUID: "author-uuid-1"}
		)

		notesRepository.On("GetByCorrelationUUIDAndLanguage", mock.Anything, "correlation-1", "EN").Once().Return(existing, nil)
		notesRepository.On("DeleteByCorrelationUUIDAndLanguage", mock.Anything, "correlation-1", "EN").Once().Return(nil)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository)
		err := usecase.Execute(context.Background(), &request)

		assert.NoError(t, err)
	})

	t.Run("refuses to delete another author's note when scoped to the owner", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository notes.MockNotesRepository

			existing = note.Note{UUID: "note-uuid-1", AuthorUUID: "someone-else"}
			request  = Request{CorrelationUUID: "correlation-1", LanguageCode: "EN", OwnerUUID: "author-uuid-1"}
		)

		notesRepository.On("GetByCorrelationUUIDAndLanguage", mock.Anything, "correlation-1", "EN").Once().Return(existing, nil)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository)
		err := usecase.Execute(context.Background(), &request)

		notesRepository.AssertNotCalled(t, "DeleteByCorrelationUUIDAndLanguage")

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
