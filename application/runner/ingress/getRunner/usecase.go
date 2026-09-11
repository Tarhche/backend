package getRunner

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
)

type UseCase struct {
	registry ingress.Registry
}

func NewUseCase(registry ingress.Registry) *UseCase {
	return &UseCase{
		registry: registry,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	runner, err := uc.registry.Get(ctx, request.ID)
	if err != nil {
		return nil, err
	}

	return NewResponse(&runner), nil
}
