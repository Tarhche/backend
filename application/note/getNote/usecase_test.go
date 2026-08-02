package getnote

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

const correlationUUID = "3d6b0d70-2f7a-4c5f-9a51-1c48f2a4c2b1"

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns a published note", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			author = user.User{UUID: "author-uuid-1", Name: "author-name-1"}
			n      = note.Note{
				UUID:            "note-uuid-1",
				CorrelationUUID: correlationUUID,
				Body:            "note-body-1",
				Tags:            []string{"tag-1", "tag-2"},
				AuthorUUID:      "author-uuid-1",
			}
			request = Request{CorrelationUUID: correlationUUID}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		notesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(n, nil)
		notesRepository.On("GetPublishedLanguageCodes", mock.Anything, correlationUUID).Once().Return([]string{}, nil)
		defer notesRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, "author-uuid-1").Once().Return(author, nil)
		defer userRepository.AssertExpectations(t)

		languagesRepository.On("GetByCodes", mock.Anything, []string{}).Once().Return([]language.Language{}, nil)
		elementsRepository.On("Count", mock.Anything).Once().Return(uint(0), nil)

		usecase := NewUseCase(&notesRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.Equal(t, "note-body-1", response.Body)
		assert.Equal(t, []string{"tag-1", "tag-2"}, response.Tags)
		assert.Equal(t, "author-uuid-1", response.Author.UUID)
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

			request          = Request{}
			expectedResponse = Response{
				ValidationErrors: map[string]string{
					"correlation_uuid": "this field is required",
				},
			}
		)

		validator.On("Validate", &request).Once().Return(domain.ValidationErrors(expectedResponse.ValidationErrors))
		defer validator.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(context.Background(), &request)

		notesRepository.AssertNotCalled(t, "GetOnePublished")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("returns an error on getting the note", func(t *testing.T) {
		t.Parallel()

		var (
			notesRepository     notes.MockNotesRepository
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			validator           validator.MockValidator

			expectedErr = errors.New("test error")
			request     = Request{CorrelationUUID: correlationUUID}
		)

		validator.On("Validate", &request).Once().Return(nil)
		defer validator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		notesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(note.Note{}, expectedErr)
		defer notesRepository.AssertExpectations(t)

		usecase := NewUseCase(&notesRepository, &userRepository, &languagesRepository, &languageResolver, element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New()), &validator)
		response, err := usecase.Execute(context.Background(), &request)

		userRepository.AssertNotCalled(t, "GetOne")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
