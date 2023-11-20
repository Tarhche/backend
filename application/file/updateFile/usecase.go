package updatefile

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

func (uc *UseCase) UpdateArticle(request Request) (*UpdateArticleResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &UpdateArticleResponse{
			ValidationErrors: validation,
		}, nil
	}

	article := article.Article{
		UUID:  request.UUID,
		Cover: request.Cover,
		Title: request.Title,
		Body:  request.Body,
		Author: author.Author{
			UUID: request.AuthorUUID,
		},
	}

	return &UpdateArticleResponse{}, uc.articlesRepository.Save(&article)
}
