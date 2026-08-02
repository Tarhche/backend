package updatenote

import (
	"context"

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

	existing, err := uc.noteRepository.GetByCorrelationUUIDAndLanguage(ctx, request.CorrelationUUID, request.LanguageCode)
	if err != nil {
		return nil, err
	}

	// Scoped to the caller's own notes: someone else's note is treated as if it
	// doesn't exist.
	if len(request.OwnerUUID) > 0 && existing.AuthorUUID != request.OwnerUUID {
		return nil, domain.ErrNotExists
	}

	n := note.Note{
		UUID:            existing.UUID,
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

	return nil, nil
}
