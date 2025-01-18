package updatearticle

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/author"
)

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

	a := article.Article{
		UUID:        request.UUID,
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

	if _, err := uc.articleRepository.Save(&a); err != nil {
		return nil, err
	}

	return nil, nil
}
