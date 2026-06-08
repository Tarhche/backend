package getarticles

import (
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 10

type UseCase struct {
	articleRepository article.Repository
	userRepository    user.Repository
	languageResolver  resolver.Resolver
}

func NewUseCase(
	articleRepository article.Repository,
	userRepository user.Repository,
	languageResolver resolver.Resolver,
) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
		userRepository:    userRepository,
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

	totalArticles, err := uc.articleRepository.CountPublished(languageCode)
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

	a, err := uc.articleRepository.GetAllPublished(languageCode, offset, limit)
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

	publishedLanguages := make(map[string][]language.Language, len(a))
	for i := range a {
		al, err := uc.articleRepository.GetPublishedLanguages(a[i].CorrelationUUID)
		if err != nil {
			return nil, err
		}
		publishedLanguages[a[i].UUID] = al
	}

	return NewResponse(a, authors, publishedLanguages, l, totalPages, currentPage), nil
}
