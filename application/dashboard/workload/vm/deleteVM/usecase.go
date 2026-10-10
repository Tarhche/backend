package deleteVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase asks for a VM to be removed, with its disk. Its node removes it in
// its own time, and the record goes once it has; the VM's snapshots stay.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	refused, err := refusal.Of(uc.workload.DeleteVM(ctx, request.OwnerUUID, request.UUID), uc.translator)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
