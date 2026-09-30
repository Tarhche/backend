package killuserstack

import (
	"context"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase kills one of somebody's own stacks.
//
// The stack is read as theirs first, so one that is somebody else's is not
// found rather than refused, and nothing is asked of the workload about it.
type UseCase struct {
	workload workloadControlPlane.Client
}

func NewUseCase(workload workloadControlPlane.Client) *UseCase {
	return &UseCase{workload: workload}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	if _, err := uc.workload.StackOf(ctx, request.OwnerUUID, request.UUID); err != nil {
		return err
	}

	return uc.workload.KillStack(ctx, request.UUID)
}
