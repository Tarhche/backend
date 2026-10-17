package stopVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase asks for a VM to be stopped. Its disk is kept where it lives, and is
// as it was left when the VM starts again only if the disk is persistent.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	refused, err := refusal.Of(uc.workload.StopVM(ctx, request.OwnerUUID, request.UUID), uc.translator)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
