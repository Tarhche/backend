package getVMs

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase lists VMs: everybody's, or one person's own.
type UseCase struct {
	workload      workloadControlPlane.Client
	validator     domain.Validator
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(
	workload workloadControlPlane.Client,
	validator domain.Validator,
	owners *presenter.Directory,
	ingressDomain string,
) *UseCase {
	return &UseCase{
		workload:      workload,
		validator:     validator,
		owners:        owners,
		ingressDomain: ingressDomain,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	if request.Page == 0 {
		request.Page = 1
	}

	page, err := uc.workload.VMs(ctx, request.OwnerUUID, vm.Kind(request.Kind), request.Page)
	if err != nil {
		return nil, err
	}

	ownerUUIDs := make([]string, len(page.Items))
	for i := range page.Items {
		ownerUUIDs[i] = page.Items[i].OwnerUUID
	}

	owners, err := uc.owners.Of(ctx, ownerUUIDs...)
	if err != nil {
		return nil, err
	}

	return &Response{
		Items:      presenter.NewVMs(page.Items, uc.ingressDomain, owners),
		Pagination: presenter.NewPagination(page),
	}, nil
}
