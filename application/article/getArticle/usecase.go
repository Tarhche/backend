package getarticle

import (
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain"
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

func (uc *UseCase) Execute(UUID string) (*Response, error) {
	a, err := uc.articleRepository.GetOnePublished(UUID)
	if err != nil {
		return nil, err
	}

	author, err := uc.userRepository.GetOne(a.AuthorUUID)
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues([]string{
		"articles/*",
		fmt.Sprintf("articles/%s", UUID),
	})
	if err != nil {
		return nil, err
	}

	defer uc.articleRepository.IncreaseView(a.UUID, 1)

	return NewResponse(a, author, elementsResponse), nil
}
