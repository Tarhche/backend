package deleteelement

import (
	"context"

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

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.elementRepository.Delete(ctx, request.ElementUUID)
}
