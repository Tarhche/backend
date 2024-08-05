package getArticlesByHashtag

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
	currentPage := request.Page
	if currentPage == 0 {
		currentPage = 1
	}

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	hashtags := []string{request.Hashtag}

	a, err := uc.articleRepository.GetByHashtag(hashtags, offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(a, currentPage), nil
}
