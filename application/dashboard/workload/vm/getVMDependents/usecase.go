package getVMDependents

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
)

// UseCase reads what needs a VM before it is asked to be deleted: the
// containers and the stacks in it, which make it again, under its uuid and as
// it was, once it is gone, as its delete would be answered with them now.
// Nothing is deleted or asked for reading it: a workload that does not know
// the question answers that there is nothing there.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	needed, err := uc.workload.VMDependents(ctx, request.OwnerUUID, request.UUID)

	refused, err := workload.RefusalOf(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	return &Response{Needs: presenter.NewNeeds(needed)}, nil
}
