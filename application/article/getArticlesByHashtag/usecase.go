package getArticlesByHashtag

import (
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

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	hashtags := []string{request.Hashtag}

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

	totalArticles, err := uc.articleRepository.CountPublishedByHashtags(hashtags, languageCode)
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

	a, err := uc.articleRepository.GetPublishedByHashtags(hashtags, languageCode, offset, limit)
	if err != nil {
		return nil, err
	}

	userUUIDs := make([]string, len(a))
	for i := range a {
		userUUIDs[i] = a[i].AuthorUUID
	}

	authors, err := uc.userRepository.GetByUUIDs(userUUIDs)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues(
		[]string{fmt.Sprintf("/%s/hashtags/%s", languageCode, request.Hashtag)},
		languageCode,
	)
	if err != nil {
		return nil, err
	}

	publishedLanguages := make(map[string][]language.Language, len(a))
	for i := range a {
		codes, err := uc.articleRepository.GetPublishedLanguageCodes(a[i].CorrelationUUID)
		if err != nil {
			return nil, err
		}

		al, err := uc.languageRepository.GetByCodes(codes)
		if err != nil {
			return nil, err
		}
		publishedLanguages[a[i].UUID] = al
	}

	return NewResponse(a, authors, publishedLanguages, l, elementsResponse, totalPages, currentPage), nil
}
