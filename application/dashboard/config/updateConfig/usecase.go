package updateConfig

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

type UseCase struct {
	configRepository   config.Repository
	languageRepository language.Repository
	validator          domain.Validator
	translator         translator.Translator
}

func NewUseCase(
	configRepository config.Repository,
	languageRepository language.Repository,
	validator domain.Validator,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		configRepository:   configRepository,
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

	if !uc.languageRepository.Exists(request.DefaultLanguageCode) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"default_language_code": uc.translator.Translate("invalid_value"),
			},
		}, nil
	}

	c, err := uc.configRepository.GetLatestRevision()
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	c.UserDefaultRoleUUIDs = request.UserDefaultRoles
	c.DefaultLanguageCode = request.DefaultLanguageCode

	_, err = uc.configRepository.Save(&c)

	return nil, err
}
