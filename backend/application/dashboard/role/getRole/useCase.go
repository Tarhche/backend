package getrole

import "github.com/khanzadimahdi/testproject/domain/role"

type UseCase struct {
	elementRepository role.Repository
}

func NewUseCase(elementRepository role.Repository) *UseCase {
	return &UseCase{
		elementRepository: elementRepository,
	}
}

func (uc *UseCase) Execute(UUID string) (*Response, error) {
	a, err := uc.elementRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(a), nil
}
