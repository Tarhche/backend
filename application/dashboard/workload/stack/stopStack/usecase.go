package stopStack

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
)

// UseCase asks for a stack's containers to be stopped, and kept.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	refused, err := workload.RefusalOf(uc.workload.StopStack(ctx, request.OwnerUUID, request.UUID), uc.translator)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
