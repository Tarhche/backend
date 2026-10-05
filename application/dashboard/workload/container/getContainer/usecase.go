package getContainer

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stackindex"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase reads one container, from its VM's dockerd as it is now, and names
// the stack that deployed it when one did.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	container, err := uc.workload.Docker(request.OwnerUUID, request.VMUUID).Container(ctx, request.ID)

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	presented := presenter.NewContainer(container)
	if len(presented.Stack) > 0 {
		presented.StackUUID = stackindex.Of(ctx, uc.workload, request.OwnerUUID, request.VMUUID).Of(presented.Stack)
	}

	return &Response{Container: &presented}, nil
}
