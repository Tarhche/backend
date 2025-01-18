package getelement

import "github.com/khanzadimahdi/testproject/domain/element"

type UseCase struct {
	elementRepository element.Repository
}

func NewUseCase(elementRepository element.Repository) *UseCase {
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
