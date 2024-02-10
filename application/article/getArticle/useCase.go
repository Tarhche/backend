package getarticle

import (
	"fmt"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
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
	a, err := uc.articleRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	e, err := uc.elementRepository.GetByVenues([]string{fmt.Sprintf("articles/%s", UUID)})
	if err != nil {
		return nil, err
	}

	defer uc.articleRepository.IncreaseView(a.UUID, 1)

	return NewGetArticleReponse(a, e), nil
}
