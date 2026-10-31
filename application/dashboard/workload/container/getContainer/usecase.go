package getContainer

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
)

// UseCase reads one container, from its VM's dockerd as it is now, and names
// the stack whose graph made it when one did, and whoever asks can see it.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	container, err := uc.workload.Docker(request.OwnerUUID, request.VMUUID).Container(ctx, request.ID)

	refused, err := workload.RefusalOf(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	presented := presenter.NewContainer(container)
	if len(container.Stack) > 0 {
		workload.StackIndexOf(ctx, uc.workload, request.OwnerUUID, request.VMUUID).NameStack(&presented, container.Stack)
	}

	return &Response{Container: &presented}, nil
}
