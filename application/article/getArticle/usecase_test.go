package getarticle

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("returns an article", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			mockComponent      component.MockComponent

			articleUUID  string   = "test-uuid"
			venues       []string = []string{fmt.Sprintf("articles/%s", articleUUID)}
			increaseView uint     = 1

			a  = article.Article{UUID: articleUUID}
			va = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
			}
			i = []component.Item{
				{UUID: va[0].UUID},
				{UUID: va[1].UUID},
				{UUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].UUID,
				i[1].UUID,
				i[2].UUID,
			}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		articlesRepository.On("IncreaseView", articleUUID, increaseView).Once().Return(nil)
		articlesRepository.On("GetByUUIDs", u).Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []element.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		response, err := NewUseCase(&articlesRepository, &elementsRepository).Execute("test-uuid")

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("error on getting article", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository

			articleUUID string = "test-uuid"
			expectedErr        = domain.ErrNotExists
		)

		a := article.Article{}

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
		response, err := usecase.Execute("test-uuid")

		elementsRepository.AssertNotCalled(t, "GetByVenues")
		articlesRepository.AssertNotCalled(t, "GetByUUIDs")
		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting elements", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository

			articleUUID string   = "test-uuid"
			venues      []string = []string{fmt.Sprintf("articles/%s", articleUUID)}
			expectedErr          = domain.ErrNotExists
			a                    = article.Article{UUID: articleUUID}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		defer articlesRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", venues).Once().Return(nil, expectedErr)
		defer elementsRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
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
			mockComponent      component.MockComponent

			articleUUID string   = "test-uuid"
			venues      []string = []string{fmt.Sprintf("articles/%s", articleUUID)}
			expectedErr          = domain.ErrNotExists

			a  = article.Article{UUID: articleUUID}
			va = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
			}
			i = []component.Item{
				{UUID: va[0].UUID},
				{UUID: va[1].UUID},
				{UUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].UUID,
				i[1].UUID,
				i[2].UUID,
			}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		articlesRepository.On("GetByUUIDs", u).Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []element.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		response, err := NewUseCase(&articlesRepository, &elementsRepository).Execute("test-uuid")

		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on increasing template count is not reflected on response", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			mockComponent      component.MockComponent

			articleUUID  string   = "test-uuid"
			venues       []string = []string{fmt.Sprintf("articles/%s", articleUUID)}
			increaseView uint     = 1
			expectedErr           = domain.ErrNotExists

			a  = article.Article{UUID: articleUUID}
			va = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
			}
			i = []component.Item{
				{UUID: va[0].UUID},
				{UUID: va[1].UUID},
				{UUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].UUID,
				i[1].UUID,
				i[2].UUID,
			}
		)

		articlesRepository.On("GetOnePublished", articleUUID).Once().Return(a, nil)
		articlesRepository.On("IncreaseView", articleUUID, increaseView).Once().Return(expectedErr)
		articlesRepository.On("GetByUUIDs", u).Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []element.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		response, err := NewUseCase(&articlesRepository, &elementsRepository).Execute("test-uuid")

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})
}
