package createarticle

import (
	"github.com/khanzadimahdi/testproject.git/domain/article"
	"github.com/khanzadimahdi/testproject.git/domain/author"
)

type UseCase struct {
	articlesRepository article.Repository
}

func NewUseCase(articlesRepository article.Repository) *UseCase {
	return &UseCase{
		articlesRepository: articlesRepository,
	}
}

func (uc *UseCase) CreateArticle(request Request) (*CreateArticleResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &CreateArticleResponse{
			ValidationErrors: validation,
		}, nil
	}

	article := article.Article{
		Cover: request.Cover,
		Title: request.Title,
		Body:  request.Body,
		Author: author.Author{
			UUID: request.AuthorUUID,
		},
	}

	return &CreateArticleResponse{}, uc.articlesRepository.Save(&article)
}
