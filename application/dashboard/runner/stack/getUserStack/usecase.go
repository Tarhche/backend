package getuserstack

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Response struct {
	presenter.Stack
}

// Request is one of somebody's own stacks to show.
type Request struct {
	UUID string `json:"-"`

	// OwnerUUID is whose stack this has to be. It is not asked for: it is who
	// is asking, filled in by the handler.
	OwnerUUID string `json:"-"`
}

// UseCase reads one of somebody's own stacks and the services in it. One that
// is somebody else's is not found rather than refused.
type UseCase struct {
	runner        runnerManager.Client
	owners        *owners.Directory
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ownerDirectory *owners.Directory, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := uc.runner.StackOf(ctx, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	people, err := uc.owners.Of(ctx, s.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{Stack: presenter.NewStack(s, uc.ingressDomain, people)}, nil
}
