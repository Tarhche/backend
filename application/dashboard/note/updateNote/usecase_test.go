package updatenote

import (
	"context"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/notes"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("updates a note", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			languagesRepository languages.MockLanguagesRepository
			validator           validator.MockValidator
			translator          translator.TranslatorMock

			existing = note.Note{UUID: "note-uuid-1", CorrelationUUID: "correlation-1", AuthorUUID: "author-uuid-1"}
			request  = Request{
				CorrelationUUID: "correlation-1",
				Body:            "updated body",
				AuthorUUID:      "author-uuid-1",
				LanguageCode:    "EN",
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languagesRepository.On("Exists", mock.Anything, "EN").Once().Return(true)
		defer languagesRepository.AssertExpectations(t)

		notesRepository.On("GetByCorrelationUUIDAndLanguage", mock.Anything, "correlation-1", "EN").Once().Return(existing, nil)
		notesRepository.On("Save", mock.Anything, mock.Anything).Once().Return("note-uuid-1", nil)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &languagesRepository, &validator, &translator)
		response, err := usecase.Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.Nil(t, response)
	})

	t.Run("refuses to update another author's note when scoped to the owner", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			languagesRepository languages.MockLanguagesRepository
			validator           validator.MockValidator
			translator          translator.TranslatorMock

			existing = note.Note{UUID: "note-uuid-1", CorrelationUUID: "correlation-1", AuthorUUID: "someone-else"}
			request  = Request{
				CorrelationUUID: "correlation-1",
				Body:            "updated body",
				AuthorUUID:      "author-uuid-1",
				OwnerUUID:       "author-uuid-1",
				LanguageCode:    "EN",
			}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languagesRepository.On("Exists", mock.Anything, "EN").Once().Return(true)
		defer languagesRepository.AssertExpectations(t)

		notesRepository.On("GetByCorrelationUUIDAndLanguage", mock.Anything, "correlation-1", "EN").Once().Return(existing, nil)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &languagesRepository, &validator, &translator)
		response, err := usecase.Execute(context.Background(), &request)

		notesRepository.AssertNotCalled(t, "Save")

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Nil(t, response)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			languagesRepository languages.MockLanguagesRepository
			validator           validator.MockValidator
			translator          translator.TranslatorMock

			request          = Request{}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"body": "this field is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &languagesRepository, &validator, &translator)
		response, err := usecase.Execute(context.Background(), &request)

		notesRepository.AssertNotCalled(t, "GetByCorrelationUUIDAndLanguage")
		notesRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})
}
