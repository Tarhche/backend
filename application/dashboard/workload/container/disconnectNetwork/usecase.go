package disconnectNetwork

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
)

// UseCase detaches a container from a network.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	refused, err := workload.RefusalOf(
		uc.workload.Docker(request.OwnerUUID, request.VMUUID).DisconnectNetwork(ctx, request.Network, request.ID, false),
		uc.translator,
	)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
