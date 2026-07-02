package createlanguage

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

type UseCase struct {
	languageRepository language.Repository
	validator          domain.Validator
	translator         translator.Translator
}

func NewUseCase(languageRepository language.Repository, validator domain.Validator, translator translator.Translator) *UseCase {
	return &UseCase{
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

	if uc.languageRepository.Exists(ctx, request.Code) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"code": uc.translator.Translate("already_exists"),
			},
		}, nil
	}

	l := language.Language{
		Code: request.Code,
		Name: request.Name,
	}

	code, err := uc.languageRepository.Save(ctx, &l)
	if err != nil {
		return nil, err
	}

	return &Response{Code: code}, nil
}
