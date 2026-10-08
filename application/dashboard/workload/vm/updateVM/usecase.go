package updateVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// UseCase changes a VM. Only what the request carries is changed, so renaming
// a VM never restarts it.
type UseCase struct {
	workload      workloadControlPlane.Client
	validator     domain.Validator
	translator    translator.Translator
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(
	workload workloadControlPlane.Client,
	validator domain.Validator,
	translator translator.Translator,
	owners *presenter.Directory,
	ingressDomain string,
) *UseCase {
	return &UseCase{
		workload:      workload,
		validator:     validator,
		translator:    translator,
		owners:        owners,
		ingressDomain: ingressDomain,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	updated, err := uc.workload.UpdateVM(ctx, request.OwnerUUID, request.UUID, update(request))

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	owners, err := uc.owners.Of(ctx, updated.OwnerUUID)
	if err != nil {
		return nil, err
	}

	presented := presenter.NewVM(updated, uc.ingressDomain, owners)

	return &Response{VM: &presented}, nil
}

// update is the change as the workload is asked for it: what the request left
// out stays nil, which is what leaves it as it is.
func update(request *Request) workloadControlPlane.VMUpdate {
	var change workloadControlPlane.VMUpdate

	change.Name = request.Name

	if request.LifetimeSeconds != nil {
		lifetime := input.LifetimeOf(*request.LifetimeSeconds)
		change.Lifetime = &lifetime
	}

	if request.Ports != nil {
		ports := input.PortsOf(*request.Ports)
		if ports == nil {
			ports = []port.Port{}
		}

		change.Ports = &ports
	}

	if request.Network != nil {
		network := request.Network.VM()
		change.Network = &network
	}

	if request.Resources != nil {
		resources := request.Resources.VM()
		change.Resources = &resources
	}

	return change
}
