package deleterole

import (
	"github.com/khanzadimahdi/testproject/domain/role"
)

type UseCase struct {
	elementRepository role.Repository
}

func NewUseCase(elementRepository role.Repository) *UseCase {
	return &UseCase{
		elementRepository: elementRepository,
	}
}

func (uc *UseCase) Execute(request Request) error {
	return uc.elementRepository.Delete(request.RoleUUID)
}
