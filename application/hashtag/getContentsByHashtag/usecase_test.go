package getContentsByHashtag

import (
	"context"
	"errors"
	"testing"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
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

// mocks bundles the collaborators every subtest wires up.
type mocks struct {
	articles  articles.MockArticlesRepository
	notes     notes.MockNotesRepository
	elements  elements.MockElementsRepository
	users     users.MockUsersRepository
	languages languages.MockLanguagesRepository
	resolver  resolver.MockResolver
	validator validator.MockValidator
}

func (m *mocks) useCase() *UseCase {
	return NewUseCase(
		&m.articles,
		&m.notes,
		&m.users,
		&m.languages,
		&m.resolver,
		element.NewRetriever(&m.articles, &m.elements, &m.users, matcher.New()),
		&m.validator,
	)
}

// expectLanguage sets up the language resolution every successful run performs.
func (m *mocks) expectLanguage() {
	m.resolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
	m.resolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
}

// expectCounts sets up the per-tab totals, which every successful run reads to
// label the tabs and size the selected tab's pagination.
func (m *mocks) expectCounts(hashtag string, totalArticles, totalNotes uint) {
	m.articles.On("CountPublishedByHashtags", mock.Anything, []string{hashtag}, "EN").Once().Return(totalArticles, nil)
	m.notes.On("CountPublishedByHashtags", mock.Anything, []string{hashtag}, "EN").Once().Return(totalNotes, nil)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	const hashtag = "test-hashtag"

	t.Run("defaults to the articles tab", func(t *testing.T) {
		t.Parallel()

		var (
			m = mocks{}
			a = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
				{UUID: "test-article-2", AuthorUUID: "author-uuid-2"},
			}
			u       = []user.User{{UUID: "author-uuid-1"}, {UUID: "author-uuid-2"}}
			request = Request{Page: 1, Hashtag: hashtag}
		)

		m.validator.On("Validate", &request).Once().Return(nil)
		m.expectLanguage()
		m.expectCounts(hashtag, uint(len(a)), 3)

		m.articles.On("GetPublishedByHashtags", mock.Anything, []string{hashtag}, "EN", uint(0), uint(10)).Once().Return(a, nil)
		m.articles.On("GetPublishedLanguageCodes", mock.Anything, mock.Anything).Return([]string{}, nil)
		m.elements.On("Count", mock.Anything).Once().Return(uint(0), nil)
		m.users.On("GetByUUIDs", mock.Anything, []string{"author-uuid-1", "author-uuid-2"}).Once().Return(u, nil)
		m.languages.On("GetByCodes", mock.Anything, []string{}).Return([]language.Language{}, nil)

		response, err := m.useCase().Execute(context.Background(), &request)

		// The notes tab is only counted, never fetched.
		m.notes.AssertNotCalled(t, "GetPublishedByHashtags")

		assert.NoError(t, err)
		assert.Equal(t, TypeArticle, response.Type)
		assert.Equal(t, totalsResponse{Articles: 2, Notes: 3}, response.Totals)
		assert.Len(t, response.Items, 2)
		assert.Equal(t, TypeArticle, response.Items[0].Type)
	})

	t.Run("returns notes when the notes tab is requested", func(t *testing.T) {
		t.Parallel()

		var (
			m = mocks{}
			n = []note.Note{
				{UUID: "test-note-1", AuthorUUID: "author-uuid-1", Body: "note-body-1", Tags: []string{"tag-1"}},
			}
			u       = []user.User{{UUID: "author-uuid-1"}}
			request = Request{Page: 1, Hashtag: hashtag, Type: TypeNote}
		)

		m.validator.On("Validate", &request).Once().Return(nil)
		m.expectLanguage()
		m.expectCounts(hashtag, 5, uint(len(n)))

		m.notes.On("GetPublishedByHashtags", mock.Anything, []string{hashtag}, "EN", uint(0), uint(10)).Once().Return(n, nil)
		m.notes.On("GetPublishedLanguageCodes", mock.Anything, mock.Anything).Return([]string{}, nil)
		m.elements.On("Count", mock.Anything).Once().Return(uint(0), nil)
		m.users.On("GetByUUIDs", mock.Anything, []string{"author-uuid-1"}).Once().Return(u, nil)
		m.languages.On("GetByCodes", mock.Anything, []string{}).Return([]language.Language{}, nil)

		response, err := m.useCase().Execute(context.Background(), &request)

		m.articles.AssertNotCalled(t, "GetPublishedByHashtags")

		assert.NoError(t, err)
		assert.Equal(t, TypeNote, response.Type)
		assert.Equal(t, "note-body-1", response.Items[0].Body)
		assert.Equal(t, []string{"tag-1"}, response.Items[0].Tags)
	})

	t.Run("falls back to notes when the hashtag has no articles", func(t *testing.T) {
		t.Parallel()

		var (
			m       = mocks{}
			n       = []note.Note{{UUID: "test-note-1", AuthorUUID: "author-uuid-1"}}
			request = Request{Page: 1, Hashtag: hashtag}
		)

		m.validator.On("Validate", &request).Once().Return(nil)
		m.expectLanguage()
		m.expectCounts(hashtag, 0, uint(len(n)))

		m.notes.On("GetPublishedByHashtags", mock.Anything, []string{hashtag}, "EN", uint(0), uint(10)).Once().Return(n, nil)
		m.notes.On("GetPublishedLanguageCodes", mock.Anything, mock.Anything).Return([]string{}, nil)
		m.elements.On("Count", mock.Anything).Once().Return(uint(0), nil)
		m.users.On("GetByUUIDs", mock.Anything, mock.Anything).Once().Return([]user.User{}, nil)
		m.languages.On("GetByCodes", mock.Anything, []string{}).Return([]language.Language{}, nil)

		response, err := m.useCase().Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.Equal(t, TypeNote, response.Type)
		assert.Equal(t, uint(1), response.Pagination.TotalPages)
	})

	t.Run("stays on articles when the hashtag has neither", func(t *testing.T) {
		t.Parallel()

		var (
			m       = mocks{}
			request = Request{Page: 1, Hashtag: hashtag}
		)

		m.validator.On("Validate", &request).Once().Return(nil)
		m.expectLanguage()
		m.expectCounts(hashtag, 0, 0)

		m.articles.On("GetPublishedByHashtags", mock.Anything, []string{hashtag}, "EN", uint(0), uint(10)).Once().Return([]article.Article{}, nil)
		m.elements.On("Count", mock.Anything).Once().Return(uint(0), nil)
		m.users.On("GetByUUIDs", mock.Anything, []string{}).Once().Return([]user.User{}, nil)

		response, err := m.useCase().Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.Equal(t, TypeArticle, response.Type)
		assert.Empty(t, response.Items)
		assert.Equal(t, uint(0), response.Pagination.TotalPages)
	})

	t.Run("paginates the selected tab on its own count", func(t *testing.T) {
		t.Parallel()

		var (
			m = mocks{}
			// 25 notes over a page size of 10 is three pages, regardless of how
			// many articles carry the same hashtag.
			request = Request{Page: 3, Hashtag: hashtag, Type: TypeNote}
		)

		m.validator.On("Validate", &request).Once().Return(nil)
		m.expectLanguage()
		m.expectCounts(hashtag, 99, 25)

		m.notes.On("GetPublishedByHashtags", mock.Anything, []string{hashtag}, "EN", uint(20), uint(10)).Once().Return([]note.Note{}, nil)
		m.elements.On("Count", mock.Anything).Once().Return(uint(0), nil)
		m.users.On("GetByUUIDs", mock.Anything, []string{}).Once().Return([]user.User{}, nil)

		response, err := m.useCase().Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.Equal(t, uint(3), response.Pagination.TotalPages)
		assert.Equal(t, uint(3), response.Pagination.CurrentPage)
		assert.Equal(t, totalsResponse{Articles: 99, Notes: 25}, response.Totals)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			m       = mocks{}
			request = Request{Page: 1, Hashtag: hashtag}

			expectedResponse = Response{
				ValidationErrors: domain.ValidationErrors{
					"hashtag": "this field is required",
				},
			}
		)

		m.validator.On("Validate", &request).Once().Return(expectedResponse.ValidationErrors)

		response, err := m.useCase().Execute(context.Background(), &request)

		m.articles.AssertNotCalled(t, "CountPublishedByHashtags")
		m.notes.AssertNotCalled(t, "CountPublishedByHashtags")
		m.users.AssertNotCalled(t, "GetByUUIDs")

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		t.Parallel()

		var (
			m           = mocks{}
			expectedErr = errors.New("test error")
			request     = Request{Page: 1, Hashtag: hashtag}
		)

		m.validator.On("Validate", &request).Once().Return(nil)
		m.expectLanguage()
		m.articles.On("CountPublishedByHashtags", mock.Anything, []string{hashtag}, "EN").Once().Return(uint(0), expectedErr)

		response, err := m.useCase().Execute(context.Background(), &request)

		m.articles.AssertNotCalled(t, "GetPublishedByHashtags")
		m.users.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		t.Parallel()

		var (
			m           = mocks{}
			expectedErr = errors.New("test error")
			request     = Request{Page: 1, Hashtag: hashtag}
		)

		m.validator.On("Validate", &request).Once().Return(nil)
		m.expectLanguage()
		m.expectCounts(hashtag, 5, 0)
		m.articles.On("GetPublishedByHashtags", mock.Anything, []string{hashtag}, "EN", uint(0), uint(10)).Once().Return(nil, expectedErr)

		response, err := m.useCase().Execute(context.Background(), &request)

		m.users.AssertNotCalled(t, "GetByUUIDs")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("returns an error on getting authors", func(t *testing.T) {
		t.Parallel()

		var (
			m           = mocks{}
			expectedErr = errors.New("test error")
			a           = []article.Article{{UUID: "test-article-1", AuthorUUID: "author-uuid-1"}}
			request     = Request{Page: 1, Hashtag: hashtag}
		)

		m.validator.On("Validate", &request).Once().Return(nil)
		m.expectLanguage()
		m.expectCounts(hashtag, 1, 0)
		m.articles.On("GetPublishedByHashtags", mock.Anything, []string{hashtag}, "EN", uint(0), uint(10)).Once().Return(a, nil)
		m.users.On("GetByUUIDs", mock.Anything, []string{"author-uuid-1"}).Once().Return(nil, expectedErr)

		response, err := m.useCase().Execute(context.Background(), &request)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
