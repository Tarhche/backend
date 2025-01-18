package updateConfig

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
)

type UseCase struct {
	configRepository config.Repository
	validator        domain.Validator
}

func NewUseCase(
	configRepository config.Repository,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		configRepository: configRepository,
		validator:        validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	c, err := uc.configRepository.GetLatestRevision()
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	c.UserDefaultRoleUUIDs = request.UserDefaultRoles

	_, err = uc.configRepository.Save(&c)

	return nil, err
}
