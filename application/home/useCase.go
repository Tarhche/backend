package home

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

func (uc *UseCase) Execute() (*Response, error) {
	articles, err := uc.articleRepository.GetAll(0, 10)
	if err != nil {
		return nil, err
	}

	return NewResponse(articles, articles, articles), err
}
