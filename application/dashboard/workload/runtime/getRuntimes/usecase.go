// Package getRuntimes lists the classes a task may be run with, for whoever is
// choosing one: the dashboard's task and stack forms are built from it.
//
// Which classes there are, which is the default and what each can do right
// now is the workload's to say, so this asks it and presents what it said.
// Nothing here narrows the list by who is asking: the workload permissions
// cover every class, and there is no permission per class yet.
package getRuntimes

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase lists the runtime classes of the whole workload.
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
