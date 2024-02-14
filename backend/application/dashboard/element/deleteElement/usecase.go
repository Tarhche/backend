package deleteelement

import (
	"github.com/khanzadimahdi/testproject/domain/element"
)

type UseCase struct {
	elementRepository element.Repository
}

func NewUseCase(elementRepository element.Repository) *UseCase {
	return &UseCase{
		elementRepository: elementRepository,
	}
}

func (uc *UseCase) DeleteElement(request Request) error {
	return uc.elementRepository.Delete(request.ElementUUID)
}
