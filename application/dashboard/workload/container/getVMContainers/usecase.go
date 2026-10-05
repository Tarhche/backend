package getVMContainers

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stackindex"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// UseCase lists one Docker VM's containers, stopped ones too: a container that
// exited is one somebody may want to start again, or remove. One a stack
// deployed names the stack.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	containers, err := uc.workload.Docker(request.OwnerUUID, request.VMUUID).Containers(ctx, docker.ContainerFilter{All: true})

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	items := presenter.NewContainers(containers)
	stackindex.Of(ctx, uc.workload, request.OwnerUUID, request.VMUUID).Link(items)

	return &Response{Items: items}, nil
}
