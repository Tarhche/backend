package updatearticle

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

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

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if !uc.languageRepository.Exists(request.LanguageCode) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"language_code": uc.translator.Translate("invalid_value"),
			},
		}, nil
	}

	existing, err := uc.articleRepository.GetByCorrelationUUIDAndLanguage(request.CorrelationUUID, request.LanguageCode)
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
		AuthorUUID:      request.AuthorUUID,
		Tags:            request.Tags,
		LanguageCode:    request.LanguageCode,
		CorrelationUUID: request.CorrelationUUID,
		ViewCount:       existing.ViewCount,
	}

	if _, err := uc.articleRepository.Save(&a); err != nil {
		return nil, err
	}

	return nil, nil
}
