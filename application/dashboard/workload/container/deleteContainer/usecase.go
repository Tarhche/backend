package deleteContainer

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/translator"
	refusal "github.com/khanzadimahdi/testproject/infrastructure/workload"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
)

// UseCase removes a container. Its volumes stay: they are the VM's, and
// another container may mount them.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	refused, err := refusal.Of(
		uc.workload.Docker(request.OwnerUUID, request.VMUUID).RemoveContainer(ctx, request.ID, request.Force),
		uc.translator,
	)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
