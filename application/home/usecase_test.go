package home

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			mockComponent      component.MockComponent

			a = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
				{UUID: "test-article-3"},
			}

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

		articlesRepository.On("GetMostViewed", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(3)).Once().Return(a, nil)
		articlesRepository.On("GetByUUIDs", u).Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []element.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", []string{"home"}).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
		response, err := usecase.Execute()

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err)
	})

	t.Run("error on getting most viewed articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", uint(4)).Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
		response, err := usecase.Execute()

		articlesRepository.AssertNotCalled(t, "GetAllPublished")
		articlesRepository.AssertNotCalled(t, "GetByUUIDs")
		elementsRepository.AssertNotCalled(t, "GetByVenues")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting all published articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository

			a = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
				{UUID: "test-article-3"},
			}

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(3)).Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
		response, err := usecase.Execute()

		elementsRepository.AssertNotCalled(t, "GetByVenues")
		articlesRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting elements", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository

			a = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
				{UUID: "test-article-3"},
			}

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(3)).Return(a, nil)
		defer articlesRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", []string{"home"}).Once().Return(nil, expectedErr)
		defer elementsRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
		response, err := usecase.Execute()

		articlesRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting element articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			mockComponent      component.MockComponent

			a = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
				{UUID: "test-article-3"},
			}

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

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(3)).Once().Return(a, nil)
		articlesRepository.On("GetByUUIDs", u).Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []element.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", []string{"home"}).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &elementsRepository)
		response, err := usecase.Execute()

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})
}
