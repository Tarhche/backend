package getContainers

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
)

// UseCase lists the containers across every running Docker VM, each with the
// VM it was found in and the stack whose graph made it, when one did and
// whoever asks can see it. They are read from each VM's dockerd as they are
// now: nothing keeps a list of them anywhere else.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	containers, err := uc.workload.Containers(ctx, request.OwnerUUID, request.VMUUID)

	refused, err := workload.RefusalOf(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	items := presenter.NewVMContainers(containers)

	index := workload.StackIndexOf(ctx, uc.workload, request.OwnerUUID, request.VMUUID)
	for i := range items {
		index.NameStack(&items[i].Container, containers[i].Stack)
	}

	return &Response{Items: items}, nil
}
