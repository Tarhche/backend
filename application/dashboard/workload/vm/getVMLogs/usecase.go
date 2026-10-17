package getVMLogs

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase reads the tail of what a VM has written, from its node as it is
// now: nothing keeps a VM's log anywhere else.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	lines, err := uc.workload.VMLogs(ctx, request.OwnerUUID, request.UUID, vm.LogOptions{
		Since: request.Since,
		Tail:  request.Tail,
	})

	refused, err := refusal.Of(err, uc.translator)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	return &Response{
		Items:     presenter.NewVMLogLines(lines),
		Truncated: presenter.Truncated(len(lines), request.Tail),
	}, nil
}
