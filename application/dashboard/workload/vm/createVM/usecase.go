package createVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase asks the workload for a VM. The workload owns a VM's life, and holds
// the bounds and quotas it is weighed against; this decides only whether the
// request is well formed before passing it on.
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

	created, err := uc.workload.CreateVM(ctx, request.OwnerUUID, workloadControlPlane.VMRequest{
		Name:           request.Name,
		Kind:           vm.Kind(request.Kind),
		Image:          request.Image,
		Resources:      request.Resources.VM(),
		Ports:          input.PortsOf(request.Ports),
		Network:        request.Network.VM(),
		PersistentDisk: request.PersistentDisk,
		Lifetime:       input.LifetimeOf(request.LifetimeSeconds),
		SnapshotUUID:   request.SnapshotUUID,
	})

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	owners, err := uc.owners.Of(ctx, created.OwnerUUID)
	if err != nil {
		return nil, err
	}

	presented := presenter.NewVM(created, uc.ingressDomain, owners)

	return &Response{VM: &presented}, nil
}
