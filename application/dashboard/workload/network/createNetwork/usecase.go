package createNetwork

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase creates a docker network in a Docker VM, for its containers to meet
// each other on.
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

	created, err := uc.workload.Docker(request.OwnerUUID, request.VMUUID).CreateNetwork(ctx, request.Spec())

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	presented := presenter.NewDockerNetwork(created)

	return &Response{DockerNetwork: &presented}, nil
}
