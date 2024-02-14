package deletearticle

import (
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

func (uc *UseCase) DeleteArticle(request Request) error {
	return uc.articleRepository.Delete(request.ArticleUUID)
}
