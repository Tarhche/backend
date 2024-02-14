package home

import (
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
)

type UseCase struct {
	articleRepository article.Repository
	elementRepository element.Repository
}

func NewUseCase(
	articleRepository article.Repository,
	elementRepository element.Repository,
) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
		elementRepository: elementRepository,
	}
}

func (uc *UseCase) Execute() (*Response, error) {
	popular, err := uc.articleRepository.GetMostViewed(4)
	if err != nil {
		return nil, err
	}

	all, err := uc.articleRepository.GetAll(0, 3)
	if err != nil {
		return nil, err
	}

	elements, elementsArticles, err := uc.elements()
	if err != nil {
		return nil, err
	}

	return NewResponse(all, popular, elements, elementsArticles), err
}

func (uc *UseCase) elements() ([]element.Element, []article.Article, error) {
	elements, err := uc.elementRepository.GetByVenues([]string{"home"})
	if err != nil {
		return nil, nil, err
	}

	items := make([]component.Item, 0, len(elements))
	for i := range elements {
		items = append(items, elements[i].Body.Items()...)
	}

	uuids := make([]string, len(items))
	for i := range items {
		uuids[i] = items[i].UUID
	}
	articles, err := uc.articleRepository.GetByUUIDs(uuids)
	if err != nil {
		return nil, nil, err
	}

	return elements, articles, nil
}
