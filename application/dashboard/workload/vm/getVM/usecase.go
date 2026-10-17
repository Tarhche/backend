package getVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase reads one VM, with the addresses its ports are served on.
type UseCase struct {
	workload      workloadControlPlane.Client
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(workload workloadControlPlane.Client, owners *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{workload: workload, owners: owners, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	v, err := uc.workload.VM(ctx, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	owners, err := uc.owners.Of(ctx, v.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{VM: presenter.NewVM(v, uc.ingressDomain, owners)}, nil
}
