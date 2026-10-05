package getContainers

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stackindex"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase lists the containers across every running Docker VM, each with the
// VM it was found in and the stack that deployed it, when one did. They are
// read from each VM's dockerd as they are now: nothing keeps a list of them
// anywhere else.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	containers, err := uc.workload.Containers(ctx, request.OwnerUUID, request.VMUUID)

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	items := presenter.NewVMContainers(containers)

	index := stackindex.Of(ctx, uc.workload, request.OwnerUUID, request.VMUUID)
	for i := range items {
		items[i].StackUUID = index.Of(items[i].Stack)
	}

	return &Response{Items: items}, nil
}
