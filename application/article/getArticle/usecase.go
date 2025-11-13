package getarticle

import (
	"fmt"

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

func (uc *UseCase) Execute(UUID string) (*Response, error) {
	a, err := uc.articleRepository.GetOnePublished(UUID)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues([]string{fmt.Sprintf("articles/%s", UUID)})
	if err != nil {
		return nil, err
	}

	defer uc.articleRepository.IncreaseView(a.UUID, 1)

	return NewResponse(a, elementsResponse), nil
}
