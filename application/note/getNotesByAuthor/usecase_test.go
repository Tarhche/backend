package getNotesByAuthor

import (
	"context"
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/matcher"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/notes"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns an author's published notes", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			author = user.User{UUID: "author-uuid-1", Name: "author-name-1", Username: "author-1"}
			n      = []note.Note{
				{UUID: "note-uuid-1", CorrelationUUID: "note-correlation-1", Body: "note-body-1", Tags: []string{"tag-1"}},
				{UUID: "note-uuid-2", CorrelationUUID: "note-correlation-2", Body: "note-body-2"},
			}
			request = Request{Page: 1, Username: "author-1"}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock.Anything, "author-1").Once().Return(author, nil)
		defer userRepository.AssertExpectations(t)

		notesRepository.On("CountPublishedByAuthor", mock.Anything, author.UUID, "EN").Once().Return(uint(len(n)), nil)
		articlesRepository.On("CountPublishedByAuthor", mock.Anything, author.UUID, "EN").Once().Return(uint(0), nil)
		notesRepository.On("GetPublishedByAuthor", mock.Anything, author.UUID, "EN", uint(0), uint(10)).Once().Return(n, nil)
		notesRepository.On("GetPublishedLanguageCodes", mock.Anything, mock.Anything).Return([]string{}, nil)
		defer notesRepository.AssertExpectations(t)

		elementsRepository.On("Count", mock.Anything).Once().Return(uint(0), nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{}).Return([]language.Language{}, nil)

		usecase := NewUseCase(&notesRepository, &articlesRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.Len(t, response.Items, 2)
		assert.Equal(t, "note-body-1", response.Items[0].Body)
		assert.Equal(t, []string{"tag-1"}, response.Items[0].Tags)
		assert.Equal(t, author.UUID, response.Author.UUID)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			request          = Request{Page: 1}
			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"author": "this field is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)
		defer validator.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &articlesRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(context.Background(), &request)

		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		notesRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("returns an error on getting notes", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			author      = user.User{UUID: "author-uuid-1", Username: "author-1"}
			expectedErr = errors.New("test error")
			request     = Request{Page: 1, Username: "author-1"}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", mock.Anything, "author-1").Once().Return(author, nil)
		defer userRepository.AssertExpectations(t)

		notesRepository.On("CountPublishedByAuthor", mock.Anything, author.UUID, "EN").Once().Return(uint(1), nil)
		articlesRepository.On("CountPublishedByAuthor", mock.Anything, author.UUID, "EN").Once().Return(uint(0), nil)
		notesRepository.On("GetPublishedByAuthor", mock.Anything, author.UUID, "EN", uint(0), uint(10)).Once().Return(nil, expectedErr)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &articlesRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(context.Background(), &request)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
