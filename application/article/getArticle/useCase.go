package getarticle

import "github.com/khanzadimahdi/testproject.git/domain/article"

type UseCase struct {
	articlesRepository article.Repository
}

func NewUseCase(articlesRepository article.Repository) *UseCase {
	return &UseCase{
		articlesRepository: articlesRepository,
	}
}

func (uc *UseCase) GetArticle(UUID string) (*GetArticleResponse, error) {
	a, err := uc.articlesRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return NewGetArticleReponse(a), nil
}
