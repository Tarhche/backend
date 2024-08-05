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

func (uc *UseCase) Execute(UUID string) (*Response, error) {
	a, err := uc.articleRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(a), nil
}
