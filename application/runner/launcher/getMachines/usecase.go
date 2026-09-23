package getMachines

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// UseCase says which machines this host holds for one orchestrator. What is
// running is read off the host every time, so a machine whose process ended
// while nobody was looking is reported as the host sees it.
type UseCase struct {
	vmm       machine.VMM
	validator domain.Validator
}

func NewUseCase(vmm machine.VMM, validator domain.Validator) *UseCase {
	return &UseCase{
		vmm:       vmm,
		validator: validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	held, err := uc.vmm.List(ctx)
	if err != nil {
		return nil, err
	}

	machines := make([]machine.Machine, 0, len(held))
	for _, m := range held {
		if m.Owner == request.Owner {
			machines = append(machines, m)
		}
	}

	return &Response{Machines: machines}, nil
}
