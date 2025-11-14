package home

import (
	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain/article"
)

type UseCase struct {
	articleRepository article.Repository
	elementRetriever  *element.Retriever
}

func NewUseCase(
	articleRepository article.Repository,
	elementRetriever *element.Retriever,
) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
		elementRetriever:  elementRetriever,
	}
}

func (uc *UseCase) Execute() (*Response, error) {
	popular, err := uc.articleRepository.GetMostViewed(4)
	if err != nil {
		return nil, err
	}

	all, err := uc.articleRepository.GetAllPublished(0, 3)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues([]string{"home"})
	if err != nil {
		return nil, err
	}

	return NewResponse(all, popular, elementsResponse), nil
}
