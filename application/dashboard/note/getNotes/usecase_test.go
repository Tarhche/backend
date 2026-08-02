package getnotes

import (
	"context"
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/notes"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("lists every author's notes grouped by correlation uuid", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository

			correlationUUIDs = []string{"correlation-1"}
			n                = []note.Note{
				{UUID: "note-uuid-1", CorrelationUUID: "correlation-1", Body: "body-en", LanguageCode: "EN", AuthorUUID: "author-uuid-1"},
				{UUID: "note-uuid-2", CorrelationUUID: "correlation-1", Body: "body-fa", LanguageCode: "FA", AuthorUUID: "author-uuid-1"},
			}
			u       = []user.User{{UUID: "author-uuid-1", Name: "author-name-1"}}
			l       = []language.Language{{Code: "EN", Name: "English"}, {Code: "FA", Name: "Persian"}}
			request = Request{Page: 1}
		)

		notesRepository.On("CountByCorrelation", mock.Anything, "").Once().Return(uint(1), nil)
		notesRepository.On("GetCorrelationUUIDs", mock.Anything, "", uint(0), uint(20)).Once().Return(correlationUUIDs, nil)
		notesRepository.On("GetByCorrelationUUIDs", mock.Anything, correlationUUIDs, "").Once().Return(n, nil)
		defer notesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", mock.Anything, []string{"author-uuid-1", "author-uuid-1"}).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		languagesRepository.On("GetByCodes", mock.Anything, []string{"EN", "FA"}).Once().Return(l, nil)
		defer languagesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &userRepository, &languagesRepository)
		response, err := usecase.Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.Len(t, response.Items, 1)
		assert.Len(t, response.Items[0].CorrolatedItems, 2)
		assert.Equal(t, "English", response.Items[0].CorrolatedItems[0].Language.Name)
	})

	t.Run("scopes the listing to one author's own notes", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository

			request = Request{Page: 1, AuthorUUID: "author-uuid-1"}
		)

		notesRepository.On("CountByCorrelation", mock.Anything, "author-uuid-1").Once().Return(uint(0), nil)
		notesRepository.On("GetCorrelationUUIDs", mock.Anything, "author-uuid-1", uint(0), uint(20)).Once().Return([]string{}, nil)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &userRepository, &languagesRepository)
		response, err := usecase.Execute(context.Background(), &request)

		notesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.NoError(t, err)
		assert.Empty(t, response.Items)
	})

	t.Run("returns an error on counting notes", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository

			expectedErr = errors.New("test error")
			request     = Request{Page: 1}
		)

		notesRepository.On("CountByCorrelation", mock.Anything, "").Once().Return(uint(0), expectedErr)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &userRepository, &languagesRepository)
		response, err := usecase.Execute(context.Background(), &request)

		notesRepository.AssertNotCalled(t, "GetCorrelationUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
