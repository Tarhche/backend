package deleteVolume

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase removes a volume, and what was kept in it, from a Docker VM. One a
// container mounts is refused unless it is forced, or the stacks and the
// containers that need it are deleted first (Request.Cascade); it is
// answered with what needs it either way.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	deleted, err := uc.workload.DeleteVolume(ctx, request.OwnerUUID, request.VMUUID, request.Name, request.Force, request.Cascade)

	refused, err := refusal.Of(err, uc.translator)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused, Dependents: presenter.NewDependents(deleted), Recreated: deleted.Recreated}, nil
}
