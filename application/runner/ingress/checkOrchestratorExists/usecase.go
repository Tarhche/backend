package checkOrchestratorExists

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

// Execute reports whether the named runner is connected.
//
// false is an answer rather than a failure: an orchestrator that is not there is the
// ordinary state of one that has gone away, and the caller decides what to make
// of it. An error is this being unable to find out, which a registry kept
// anywhere but in memory can be, and which is not the same thing at all.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (bool, error) {
	return uc.registry.Exists(ctx, request.Name)
}
