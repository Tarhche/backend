package deleteStack

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase takes a stack down and removes it. Compose does it inside the VM in
// its own time, and the record goes once it has.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	refused, err := refusal.Of(
		uc.workload.DeleteStack(ctx, request.OwnerUUID, request.UUID, request.RemoveVolumes),
		uc.translator,
	)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
