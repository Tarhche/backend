package updateConfig

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
)

type UseCase struct {
	configRepository config.Repository
}

func NewUseCase(configRepository config.Repository) *UseCase {
	return &UseCase{
		configRepository: configRepository,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	c, err := uc.configRepository.GetLatestRevision()
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	c.UserDefaultRoleUUIDs = request.UserDefaultRoles

	if _, err = uc.configRepository.Save(&c); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
