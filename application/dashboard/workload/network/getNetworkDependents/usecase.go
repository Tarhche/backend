package getNetworkDependents

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase reads what needs a docker network of a Docker VM before it is
// asked to be deleted, the network found by its id or its name in the VM as
// its delete finds it: the containers and the stacks that need it, as its
// delete would be answered with them now. Nothing is deleted or asked for
// reading it: a workload that does not know the question answers that there
// is nothing there.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	needed, err := uc.workload.NetworkDependents(ctx, request.OwnerUUID, request.VMUUID, request.ID)

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	return &Response{Needs: presenter.NewNeeds(needed)}, nil
}
