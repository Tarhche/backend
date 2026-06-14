package home

import (
	"fmt"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	articleRepository article.Repository
	userRepository    user.Repository
	elementRetriever  *element.Retriever
	languageResolver  resolver.Resolver
}

func NewUseCase(
	articleRepository article.Repository,
	userRepository user.Repository,
	elementRetriever *element.Retriever,
	languageResolver resolver.Resolver,
) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
		userRepository:    userRepository,
		elementRetriever:  elementRetriever,
		languageResolver:  languageResolver,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	languageCode := request.LanguageCode
	if len(languageCode) == 0 {
		code, err := uc.languageResolver.DefaultCode()
		if err != nil {
			return nil, err
		}

		languageCode = code
	}

	l, err := uc.languageResolver.Resolve(languageCode)
	if err != nil {
		return nil, err
	}

	popular, err := uc.articleRepository.GetMostViewed(languageCode, 4)
	if err != nil {
		return nil, err
	}

	all, err := uc.articleRepository.GetAllPublished(languageCode, 0, 3)
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

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues(
		[]string{fmt.Sprintf("/%s/home", languageCode)},
		languageCode,
	)
	if err != nil {
		return nil, err
	}

	return NewResponse(all, popular, u, l, elementsResponse), nil
}
