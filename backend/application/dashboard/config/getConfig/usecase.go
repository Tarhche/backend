package getConfig

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

func (uc *UseCase) Execute() (*Response, error) {
	c, err := uc.configRepository.GetLatestRevision()
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	response := Response{
		Revision:         c.Revision,
		UserDefaultRoles: c.UserDefaultRoleUUIDs,
	}

	if response.UserDefaultRoles == nil {
		response.UserDefaultRoles = []string{}
	}

	return &response, nil
}
