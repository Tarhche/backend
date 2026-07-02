package getArticlesByAuthor

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
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
	validator          domain.Validator
}

func NewUseCase(
	articleRepository article.Repository,
	userRepository user.Repository,
	languageRepository language.Repository,
	languageResolver resolver.Resolver,
	elementRetriever *element.Retriever,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		articleRepository:  articleRepository,
		userRepository:     userRepository,
		languageRepository: languageRepository,
		languageResolver:   languageResolver,
		elementRetriever:   elementRetriever,
		validator:          validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

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

	author, err := uc.resolveAuthor(ctx, request)
	if err != nil {
		return nil, err
	}

	totalArticles, err := uc.articleRepository.CountPublishedByAuthor(ctx, author.UUID, languageCode)
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

	a, err := uc.articleRepository.GetPublishedByAuthor(ctx, author.UUID, languageCode, offset, limit)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues(
		ctx,
		[]string{fmt.Sprintf("/%s/authors/%s/articles", languageCode, author.UUID)},
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

	return NewResponse(author, a, publishedLanguages, l, elementsResponse, totalPages, currentPage), nil
}

func (uc *UseCase) resolveAuthor(ctx context.Context, request *Request) (user.User, error) {
	if len(request.AuthorUUID) > 0 {
		return uc.userRepository.GetOne(ctx, request.AuthorUUID)
	}

	return uc.userRepository.GetOneByIdentity(ctx, request.Username)
}
