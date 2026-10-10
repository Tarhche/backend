package deleteVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase asks for a VM to be removed, with its disk. Its node removes it in
// its own time, and the record goes once it has; the VM's snapshots stay. The
// containers in it, nothing owning them, make it again, under its uuid and as
// it was, and then themselves in it, unless they are deleted with it
// (Request.Cascade): it is answered with them either way.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	deleted, err := uc.workload.DeleteVM(ctx, request.OwnerUUID, request.UUID, request.Cascade)

	refused, err := refusal.Of(err, uc.translator)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused, Dependents: presenter.NewDependents(deleted), Recreated: deleted.Recreated}, nil
}
