package restoreVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase asks for a VM's disk to be replaced from a snapshot. The VM is
// stopped, restored and started again by its node; it keeps its uuid, its
// slug and its ports. Whether the snapshot fits it is the control plane's to
// say, since it holds both.
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
		uc.workload.RestoreVM(ctx, request.OwnerUUID, request.UUID, request.SnapshotUUID),
		uc.translator,
	)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
