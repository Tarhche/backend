package createlanguage

import (
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

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if uc.languageRepository.Exists(request.Code) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"code": "already_exists",
			},
		}, nil
	}

	l := language.Language{
		Code: request.Code,
		Name: request.Name,
	}

	code, err := uc.languageRepository.Save(&l)
	if err != nil {
		return nil, err
	}

	return &Response{Code: code}, nil
}
