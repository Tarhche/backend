package createSnapshot

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase asks for a snapshot of a VM's disk. It is taken by the VM's node in
// its own time: what comes back is a snapshot being created, which is ready
// once its archive is stored.
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

	taken, err := uc.workload.CreateSnapshot(ctx, request.OwnerUUID, request.VMUUID, request.Name)

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	owners, err := uc.owners.Of(ctx, taken.OwnerUUID)
	if err != nil {
		return nil, err
	}

	presented := presenter.NewSnapshot(taken, owners)

	return &Response{Snapshot: &presented}, nil
}
