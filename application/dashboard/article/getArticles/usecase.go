package getarticles

import (
	"github.com/khanzadimahdi/testproject/domain/article"
)

const limit = 10

type UseCase struct {
	articleRepository article.Repository
}

func NewUseCase(articleRepository article.Repository) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	totalArticles, err := uc.articleRepository.Count()
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

	a, err := uc.articleRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(a, totalPages, currentPage), nil
}
