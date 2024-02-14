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

func (uc *UseCase) GetElement(UUID string) (*GetElementResponse, error) {
	a, err := uc.elementRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return NewGetElementReponse(a), nil
}
