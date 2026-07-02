package deletearticle

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/article"
)

type UseCase struct {
	articleRepository article.Repository
}

func NewUseCase(articleRepository article.Repository) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.articleRepository.DeleteByCorrelationUUIDAndLanguage(ctx, request.CorrelationUUID, request.LanguageCode)
}
