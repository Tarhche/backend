// Package getUserRuntimes lists the classes somebody's own tasks may be run
// with, for the form that runs one of their own.
//
// It is a use case of its own, rather than the listing everybody's tasks are
// run from, because it is reached by whoever may run their own tasks, which is
// not whoever may see everybody's. What it lists is the same: there is no
// permission per class yet, so what one person may choose from is every class
// the workload allows.
package getUserRuntimes

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase lists the runtime classes the person asking may run their tasks
// with.
type UseCase struct {
	workload workloadControlPlane.Client
}

func NewUseCase(workload workloadControlPlane.Client) *UseCase {
	return &UseCase{workload: workload}
}

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	availability, err := uc.workload.Runtimes(ctx)
	if err != nil {
		return nil, err
	}

	return &Response{Items: presenter.NewRuntimes(availability)}, nil
}
