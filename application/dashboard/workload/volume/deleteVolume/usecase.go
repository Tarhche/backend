package deleteVolume

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase removes a volume, and what was kept in it, from a Docker VM. One a
// container mounts is refused unless it is forced.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	refused, err := refusal.Of(
		uc.workload.Docker(request.OwnerUUID, request.VMUUID).RemoveVolume(ctx, request.Name, request.Force),
		uc.translator,
	)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
