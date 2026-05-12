package getarticle

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	domainElement "github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("returns an article", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			mockComponent      component.MockComponent

			articleUUID  string   = "test-uuid"
			authorUUID   string   = "author-uuid"
			venues       []string = []string{"articles/*", fmt.Sprintf("articles/%s", articleUUID)}
			increaseView uint     = 1

			a = article.Article{
				UUID:       articleUUID,
				AuthorUUID: authorUUID,
			}
			au = user.User{UUID: authorUUID, Name: "author-name", Avatar: "author-avatar"}
			va = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
				{UUID: "test-article-2", AuthorUUID: "author-uuid-2"},
			}
			elementUsers = []user.User{
				{UUID: "author-uuid-1", Name: "author-name-1", Avatar: "author-avatar-1"},
				{UUID: "author-uuid-2", Name: "author-name-2", Avatar: "author-avatar-2"},
			}
			i = []component.Item{
				{ContentUUID: va[0].UUID},
				{ContentUUID: va[1].UUID},
				{ContentUUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].ContentUUID,
				i[1].ContentUUID,
				i[2].ContentUUID,
			}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		articlesRepository.On("IncreaseView", articleUUID, increaseView).Once().Return(nil)
		articlesRepository.On("GetByUUIDs", u).Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(au, nil)
		userRepository.On("GetByUUIDs", []string{"author-uuid-1", "author-uuid-2"}).Once().Return(elementUsers, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, elementRetriever).Execute("test-uuid")

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("error on getting article", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository

			articleUUID string = "test-uuid"
			expectedErr        = domain.ErrNotExists
		)

		a := article.Article{}

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, elementRetriever)
		response, err := usecase.Execute("test-uuid")

		userRepository.AssertNotCalled(t, "GetOne")
		elementsRepository.AssertNotCalled(t, "GetByVenues")
		articlesRepository.AssertNotCalled(t, "GetByUUIDs")
		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("missing author is handled gracefully", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			mockComponent      component.MockComponent

			articleUUID  string   = "test-uuid"
			authorUUID   string   = "missing-author-uuid"
			venues       []string = []string{"articles/*", fmt.Sprintf("articles/%s", articleUUID)}
			increaseView uint     = 1

			a = article.Article{
				UUID:       articleUUID,
				AuthorUUID: authorUUID,
			}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		articlesRepository.On("IncreaseView", articleUUID, increaseView).Once().Return(nil)
		articlesRepository.On("GetByUUIDs", []string{}).Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetByUUIDs", []string{}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return([]component.Item{})
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, elementRetriever).Execute("test-uuid")

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("error on getting elements", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository

			articleUUID string   = "test-uuid"
			authorUUID  string   = "author-uuid"
			venues      []string = []string{"articles/*", fmt.Sprintf("articles/%s", articleUUID)}
			expectedErr          = domain.ErrNotExists
			a                    = article.Article{
				UUID:       articleUUID,
				AuthorUUID: authorUUID,
			}
			au = user.User{UUID: authorUUID}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(au, nil)
		defer userRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", venues).Once().Return(nil, expectedErr)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, elementRetriever)
		response, err := usecase.Execute("test-uuid")

		articlesRepository.AssertNotCalled(t, "GetByUUIDs")
		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting element articles", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			mockComponent      component.MockComponent

			articleUUID string   = "test-uuid"
			authorUUID  string   = "author-uuid"
			venues      []string = []string{"articles/*", fmt.Sprintf("articles/%s", articleUUID)}
			expectedErr          = domain.ErrNotExists

			a = article.Article{
				UUID:       articleUUID,
				AuthorUUID: authorUUID,
			}
			au = user.User{UUID: authorUUID}
			va = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
			}
			i = []component.Item{
				{ContentUUID: va[0].UUID},
				{ContentUUID: va[1].UUID},
				{ContentUUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].ContentUUID,
				i[1].ContentUUID,
				i[2].ContentUUID,
			}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		articlesRepository.On("GetByUUIDs", u).Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(au, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, elementRetriever).Execute("test-uuid")

		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on increasing template count is not reflected on response", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			mockComponent      component.MockComponent

			articleUUID  string   = "test-uuid"
			authorUUID   string   = "author-uuid"
			venues       []string = []string{"articles/*", fmt.Sprintf("articles/%s", articleUUID)}
			increaseView uint     = 1
			expectedErr           = domain.ErrNotExists

			a = article.Article{
				UUID:       articleUUID,
				AuthorUUID: authorUUID,
			}
			au = user.User{UUID: authorUUID}
			va = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
			}
			i = []component.Item{
				{ContentUUID: va[0].UUID},
				{ContentUUID: va[1].UUID},
				{ContentUUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].ContentUUID,
				i[1].ContentUUID,
				i[2].ContentUUID,
			}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		articlesRepository.On("IncreaseView", articleUUID, increaseView).Once().Return(expectedErr)
		articlesRepository.On("GetByUUIDs", u).Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(au, nil)
		userRepository.On("GetByUUIDs", []string{"", ""}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, elementRetriever).Execute("test-uuid")

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})
}
