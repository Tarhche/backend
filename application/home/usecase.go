package home

import (
	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	articleRepository article.Repository
	userRepository    user.Repository
	elementRetriever  *element.Retriever
}

func NewUseCase(
	articleRepository article.Repository,
	userRepository user.Repository,
	elementRetriever *element.Retriever,
) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
		userRepository:    userRepository,
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

	userUUIDs := make([]string, 0, len(popular)+len(all))
	for i := range popular {
		userUUIDs = append(userUUIDs, popular[i].AuthorUUID)
	}
	for i := range all {
		userUUIDs = append(userUUIDs, all[i].AuthorUUID)
	}

	u, err := uc.userRepository.GetByUUIDs(userUUIDs)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues([]string{"home"})
	if err != nil {
		return nil, err
	}

	return NewResponse(all, popular, u, elementsResponse), nil
}
