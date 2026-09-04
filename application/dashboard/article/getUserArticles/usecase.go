package getuserarticles

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 20

// UseCase lists the articles one person wrote, the same way the listing of
// everybody's is paged: one entry per article, however many languages it
// exists in.
type UseCase struct {
	articleRepository  article.Repository
	userRepository     user.Repository
	languageRepository language.Repository
}

func NewUseCase(
	articleRepository article.Repository,
	userRepository user.Repository,
	languageRepository language.Repository,
) *UseCase {
	return &UseCase{
		articleRepository:  articleRepository,
		userRepository:     userRepository,
		languageRepository: languageRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	totalArticles, err := uc.articleRepository.CountByCorrelationAndAuthor(ctx, request.AuthorUUID)
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

	correlationUUIDs, err := uc.articleRepository.GetCorrelationUUIDsByAuthor(ctx, request.AuthorUUID, offset, limit)
	if err != nil {
		return nil, err
	}

	if len(correlationUUIDs) == 0 {
		return NewResponse(correlationUUIDs, nil, nil, nil, totalPages, currentPage), nil
	}

	articles, err := uc.articleRepository.GetByCorrelationUUIDs(ctx, correlationUUIDs, "")
	if err != nil {
		return nil, err
	}

	userUUIDs := make([]string, len(articles))
	languageCodes := make([]string, len(articles))
	for i := range articles {
		userUUIDs[i] = articles[i].AuthorUUID
		languageCodes[i] = articles[i].LanguageCode
	}

	authors, err := uc.userRepository.GetByUUIDs(ctx, userUUIDs)
	if err != nil {
		return nil, err
	}

	languages, err := uc.languageRepository.GetByCodes(ctx, languageCodes)
	if err != nil {
		return nil, err
	}

	return NewResponse(correlationUUIDs, articles, authors, languages, totalPages, currentPage), nil
}
