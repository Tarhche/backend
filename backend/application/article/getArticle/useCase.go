package getarticle

import (
	"fmt"

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

func (uc *UseCase) GetArticle(UUID string) (*GetArticleResponse, error) {
	a, err := uc.articleRepository.GetOnePublished(UUID)
	if err != nil {
		return nil, err
	}

	defer uc.articleRepository.IncreaseView(a.UUID, 1)

	elements, articles, err := uc.elements(a.UUID)
	if err != nil {
		return nil, err
	}

	return NewGetArticleReponse(a, elements, articles), nil
}

func (uc *UseCase) elements(UUID string) ([]element.Element, []article.Article, error) {
	elements, err := uc.elementRepository.GetByVenues([]string{fmt.Sprintf("articles/%s", UUID)})
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
