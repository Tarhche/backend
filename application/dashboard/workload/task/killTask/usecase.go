package killTask

import (
	"context"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase kills a task. The workload owns its lifecycle, so this passes the
// command on rather than deciding anything about it.
type UseCase struct {
	workload workloadControlPlane.Client
}

func NewUseCase(workload workloadControlPlane.Client) *UseCase {
	return &UseCase{workload: workload}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.workload.KillTask(ctx, request.UUID)
}
