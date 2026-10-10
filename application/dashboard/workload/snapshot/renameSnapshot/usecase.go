package renameSnapshot

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase renames a snapshot.
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

	renamed, err := uc.workload.RenameSnapshot(ctx, request.OwnerUUID, request.UUID, request.Name)

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	owners, err := uc.owners.Of(ctx, renamed.OwnerUUID)
	if err != nil {
		return nil, err
	}

	presented := presenter.NewSnapshot(renamed, owners)

	return &Response{Snapshot: &presented}, nil
}
