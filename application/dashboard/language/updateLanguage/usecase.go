package updatelanguage

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/language"
)

type UseCase struct {
	languageRepository language.Repository
	validator          domain.Validator
}

func NewUseCase(languageRepository language.Repository, validator domain.Validator) *UseCase {
	return &UseCase{
		languageRepository: languageRepository,
		validator:          validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if !uc.languageRepository.Exists(ctx, request.Code) {
		return nil, domain.ErrNotExists
	}

	l := language.Language{
		Code: request.Code,
		Name: request.Name,
	}

	if _, err := uc.languageRepository.Save(ctx, &l); err != nil {
		return nil, err
	}

	return nil, nil
}
