package getRoles

import (
	"github.com/khanzadimahdi/testproject/domain/role"
)

const limit = 10

type UseCase struct {
	elementRepository role.Repository
}

func NewUseCase(elementRepository role.Repository) *UseCase {
	return &UseCase{
		elementRepository: elementRepository,
	}
}

func (uc *UseCase) Execute(UUID string) (*Response, error) {
	roles, err := uc.elementRepository.GetByUserUUID(UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(roles), nil
}
