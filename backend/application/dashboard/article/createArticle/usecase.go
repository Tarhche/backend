package createarticle

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

func (uc *UseCase) CreateArticle(request Request) (*CreateArticleResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &CreateArticleResponse{
			ValidationErrors: validation,
		}, nil
	}

	a := article.Article{
		Cover:       request.Cover,
		Video:       request.Video,
		Title:       request.Title,
		Excerpt:     request.Excerpt,
		Body:        request.Body,
		PublishedAt: request.PublishedAt,
		Author: author.Author{
			UUID: request.AuthorUUID,
		},
		Tags: request.Tags,
	}

	uuid, err := uc.articleRepository.Save(&a)
	if err != nil {
		return nil, err
	}

	return &CreateArticleResponse{UUID: uuid}, err
}
