package getRunners

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
)

// UseCase reports every runner the ingress can currently route to. There are
// as many of them as there are worker nodes, so they come back in one answer
// rather than a page at a time.
type UseCase struct {
	registry ingress.Registry
}

func NewUseCase(registry ingress.Registry) *UseCase {
	return &UseCase{
		registry: registry,
	}
}

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	runners, err := uc.registry.All(ctx)
	if err != nil {
		return nil, err
	}

	return NewResponse(runners), nil
}
