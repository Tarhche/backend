package deleteNetwork

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase removes a docker network from a Docker VM. One a container is
// still attached to is refused, unless the stacks and the containers that
// need it are deleted first (Request.Cascade); one a stack or a container
// owns, which it would make again, it is answered with, as with anything
// that needs it.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	deleted, err := uc.workload.DeleteNetwork(ctx, request.OwnerUUID, request.VMUUID, request.ID, request.Cascade)

	refused, err := refusal.Of(err, uc.translator)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused, Dependents: presenter.NewDependents(deleted), Recreated: deleted.Recreated}, nil
}
