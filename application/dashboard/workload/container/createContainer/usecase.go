package createContainer

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase creates and starts a container in a Docker VM of the caller's.
//
// Choosing the VM — and making one, when the caller has none — is the control
// plane's, since it is the one that knows which Docker VMs they have; the
// answer says which one it went into and whether it was made for it.
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

	created, err := uc.workload.CreateContainer(ctx, request.OwnerUUID, workloadControlPlane.ContainerRequest{
		VM:        input.DockerVMChoice(request.VMUUID, request.VM),
		Container: request.Spec(),
	})

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: input.DockerVMRefused(refused)}, nil
	}

	chosen := presenter.NewChosenVM(created.VM)
	container := presenter.NewContainer(created.Container)

	return &Response{VM: &chosen, Container: &container}, nil
}
