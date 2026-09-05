package updateuserarticle

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// UseCase changes one of the articles somebody wrote.
//
// The article is looked up as theirs, so one that is somebody else's is not
// found rather than refused, and cannot be written over by asking for it by
// its correlation uuid.
type UseCase struct {
	articleRepository  article.Repository
	languageRepository language.Repository
	validator          domain.Validator
	translator         translator.Translator
}

func NewUseCase(
	articleRepository article.Repository,
	languageRepository language.Repository,
	validator domain.Validator,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		articleRepository:  articleRepository,
		languageRepository: languageRepository,
		validator:          validator,
		translator:         translator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if !uc.languageRepository.Exists(ctx, request.LanguageCode) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"language_code": uc.translator.Translate("invalid_value"),
			},
		}, nil
	}

	existing, err := uc.articleRepository.GetByCorrelationUUIDAndLanguageAndAuthor(ctx, request.CorrelationUUID, request.LanguageCode, request.AuthorUUID)
	if err != nil {
		return nil, err
	}

	a := article.Article{
		UUID:            existing.UUID,
		Cover:           request.Cover,
		Video:           request.Video,
		Title:           request.Title,
		Excerpt:         request.Excerpt,
		Body:            request.Body,
		PublishedAt:     request.PublishedAt,
		AuthorUUID:      existing.AuthorUUID,
		Tags:            request.Tags,
		LanguageCode:    request.LanguageCode,
		CorrelationUUID: request.CorrelationUUID,
		ViewCount:       existing.ViewCount,
	}

	if _, err := uc.articleRepository.Save(ctx, &a); err != nil {
		return nil, err
	}

	return nil, nil
}
