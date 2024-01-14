package getarticle

import "github.com/khanzadimahdi/testproject/domain/article"

type UseCase struct {
	articleRepository article.Repository
}

func NewUseCase(articleRepository article.Repository) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
	}
}

func (uc *UseCase) GetArticle(UUID string) (*GetArticleResponse, error) {
	a, err := uc.articleRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return NewGetArticleReponse(a), nil
}
