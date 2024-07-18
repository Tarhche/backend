package updatearticle

import (
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/author"
)

type UseCase struct {
	articleRepository article.Repository
}

func NewUseCase(articleRepository article.Repository) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
	}
}

func (uc *UseCase) UpdateArticle(request Request) (*UpdateArticleResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &UpdateArticleResponse{
			ValidationErrors: validation,
		}, nil
	}

	article := article.Article{
		UUID:        request.UUID,
		Cover:       request.Cover,
		Title:       request.Title,
		Excerpt:     request.Excerpt,
		Body:        request.Body,
		PublishedAt: request.PublishedAt,
		Author: author.Author{
			UUID: request.AuthorUUID,
		},
		Tags: request.Tags,
	}

	if _, err := uc.articleRepository.Save(&article); err != nil {
		return nil, err
	}

	return &UpdateArticleResponse{}, nil
}
