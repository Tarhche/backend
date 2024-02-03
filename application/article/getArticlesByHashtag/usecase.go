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

func (uc *UseCase) GetArticlesByHashtag(request *Request) (*GetArticlesByHashtagResponse, error) {
	currentPage := request.Page
	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	hashtags := []string{request.Hashtag}

	a, err := uc.articleRepository.GetByHashtag(hashtags, offset, limit)
	if err != nil {
		return nil, err
	}

	return NewGetArticlesByHashtagReponse(a, currentPage), nil
}
