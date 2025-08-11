package getArticlesByHashtag

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
)

const limit = 10

type UseCase struct {
	articleRepository article.Repository
	validator         domain.Validator
}

func NewUseCase(
	articleRepository article.Repository,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
		validator:         validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	hashtags := []string{request.Hashtag}

	totalArticles, err := uc.articleRepository.CountPublishedByHashtags(hashtags)
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

	a, err := uc.articleRepository.GetPublishedByHashtags(hashtags, offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(a, totalPages, currentPage), nil
}
