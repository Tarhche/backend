package deleteuserarticle

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/article"
)

// UseCase takes away one of the articles somebody wrote.
//
// The deletion is asked for as theirs, so an article that is somebody else's
// is not touched, and one that was never theirs was never there.
type UseCase struct {
	articleRepository article.Repository
}

func NewUseCase(articleRepository article.Repository) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	a, err := uc.articleRepository.GetByCorrelationUUIDAndLanguageAndAuthor(ctx, request.CorrelationUUID, request.LanguageCode, request.AuthorUUID)
	if err != nil {
		return err
	}

	return uc.articleRepository.DeleteByCorrelationUUIDAndLanguageAndAuthor(ctx, a.CorrelationUUID, a.LanguageCode, request.AuthorUUID)
}
