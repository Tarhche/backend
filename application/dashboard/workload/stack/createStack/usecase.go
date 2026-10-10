package createStack

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// UseCase deploys a compose project into a Docker VM of the caller's.
//
// The deploy itself happens after the answer: what comes back is a stack that
// is still deploying, in the VM it named or one the control plane made for it.
// That the YAML is compose at all is the control plane's to say, since it is
// the one that reads it.
type UseCase struct {
	workload   workloadControlPlane.Client
	validator  domain.Validator
	translator translator.Translator
	owners     *presenter.Directory
}

func NewUseCase(
	workload workloadControlPlane.Client,
	validator domain.Validator,
	translator translator.Translator,
	owners *presenter.Directory,
) *UseCase {
	return &UseCase{
		workload:   workload,
		validator:  validator,
		translator: translator,
		owners:     owners,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	created, err := uc.workload.CreateStack(ctx, request.OwnerUUID, workloadControlPlane.StackRequest{
		Name:    request.Name,
		Compose: request.Compose,
		VM:      input.DockerVMChoice(request.VMUUID, request.VM),
	})

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: input.DockerVMRefused(refused)}, nil
	}

	owners, err := uc.owners.Of(ctx, created.Stack.OwnerUUID)
	if err != nil {
		return nil, err
	}

	chosen := presenter.NewChosenVM(created.VM)
	deployed := presenter.NewStack(created.Stack, owners)

	return &Response{VM: &chosen, Stack: &deployed}, nil
}
