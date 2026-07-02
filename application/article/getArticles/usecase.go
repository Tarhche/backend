package getarticles

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 10

type UseCase struct {
	articleRepository  article.Repository
	userRepository     user.Repository
	languageRepository language.Repository
	languageResolver   resolver.Resolver
	elementRetriever   *element.Retriever
}

func NewUseCase(
	articleRepository article.Repository,
	userRepository user.Repository,
	languageRepository language.Repository,
	languageResolver resolver.Resolver,
	elementRetriever *element.Retriever,
) *UseCase {
	return &UseCase{
		articleRepository:  articleRepository,
		userRepository:     userRepository,
		languageRepository: languageRepository,
		languageResolver:   languageResolver,
		elementRetriever:   elementRetriever,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	languageCode := request.LanguageCode
	if len(languageCode) == 0 {
		code, err := uc.languageResolver.DefaultCode(ctx)
		if err != nil {
			return nil, err
		}

		languageCode = code
	}

	l, err := uc.languageResolver.Resolve(ctx, languageCode)
	if err != nil {
		return nil, err
	}

	totalArticles, err := uc.articleRepository.CountPublished(ctx, languageCode)
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	if currentPage == 0 {
		currentPage = 1
	}

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalArticles / limit

	if (totalPages * limit) != totalArticles {
		totalPages++
	}

	a, err := uc.articleRepository.GetAllPublished(ctx, languageCode, offset, limit)
	if err != nil {
		return nil, err
	}

	userUUIDs := make([]string, len(a))
	for i := range a {
		userUUIDs[i] = a[i].AuthorUUID
	}

	authors, err := uc.userRepository.GetByUUIDs(ctx, userUUIDs)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues(
		ctx,
		[]string{fmt.Sprintf("/%s/articles", languageCode)},
		languageCode,
	)
	if err != nil {
		return nil, err
	}

	publishedLanguages := make(map[string][]language.Language, len(a))
	for i := range a {
		codes, err := uc.articleRepository.GetPublishedLanguageCodes(ctx, a[i].CorrelationUUID)
		if err != nil {
			return nil, err
		}

		al, err := uc.languageRepository.GetByCodes(ctx, codes)
		if err != nil {
			return nil, err
		}
		publishedLanguages[a[i].UUID] = al
	}

	return NewResponse(a, authors, publishedLanguages, l, elementsResponse, totalPages, currentPage), nil
}
