package connectNetwork

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase attaches a container to a network. A network never reaches past its
// VM, so this is only ever a container meeting its neighbours.
type UseCase struct {
	workload   workloadControlPlane.Client
	validator  domain.Validator
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, validator domain.Validator, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, validator: validator, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	refused, err := refusal.Of(
		uc.workload.Docker(request.OwnerUUID, request.VMUUID).ConnectNetwork(ctx, request.Network, request.ID, request.Aliases),
		uc.translator,
	)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
