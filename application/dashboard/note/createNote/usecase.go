package createnote

import (
	"context"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

type UseCase struct {
	noteRepository     note.Repository
	languageRepository language.Repository
	validator          domain.Validator
	translator         translator.Translator
}

func NewUseCase(
	noteRepository note.Repository,
	languageRepository language.Repository,
	validator domain.Validator,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		noteRepository:     noteRepository,
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

	if len(request.CorrelationUUID) > 0 {
		exist, err := uc.noteRepository.CorrelationExist(ctx, request.CorrelationUUID)
		if err != nil {
			return nil, err
		}

		if !exist {
			return &Response{
				ValidationErrors: domain.ValidationErrors{
					"correlation_uuid": uc.translator.Translate("invalid_value"),
				},
			}, nil
		}
	}

	// Generate a new CorrelationUUID if it's not provided in the request
	if len(request.CorrelationUUID) == 0 {
		correlationUUID, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		request.CorrelationUUID = correlationUUID.String()
	}

	n := note.Note{
		Body:            request.Body,
		PublishedAt:     request.PublishedAt,
		AuthorUUID:      request.AuthorUUID,
		Tags:            request.Tags,
		LanguageCode:    request.LanguageCode,
		CorrelationUUID: request.CorrelationUUID,
	}

	if _, err := uc.noteRepository.Save(ctx, &n); err != nil {
		return nil, err
	}

	return &Response{
		CorrelationUUID: request.CorrelationUUID,
		LanguageCode:    n.LanguageCode,
	}, nil
}
