package deleteImage

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/refusal"
	"github.com/khanzadimahdi/testproject/domain/translator"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase removes an image from a Docker VM. One a container was created
// from is refused unless it is forced.
type UseCase struct {
	workload   workloadControlPlane.Client
	translator translator.Translator
}

func NewUseCase(workload workloadControlPlane.Client, translator translator.Translator) *UseCase {
	return &UseCase{workload: workload, translator: translator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	refused, err := refusal.Of(
		uc.workload.Docker(request.OwnerUUID, request.VMUUID).RemoveImage(ctx, request.ID, request.Force),
		uc.translator,
	)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
