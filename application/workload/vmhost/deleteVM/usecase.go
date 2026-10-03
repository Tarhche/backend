// Package deleteVM takes a VM away, and everything kept for it: its machine,
// ended at once if it runs, its taps and addresses, its record, its scratch
// disk and its output.
package deleteVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase deletes VMs.
type UseCase struct {
	engine    *vmhost.Engine
	validator domain.Validator
}

func NewUseCase(engine *vmhost.Engine, validator domain.Validator) *UseCase {
	return &UseCase{engine: engine, validator: validator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	if err := uc.engine.Delete(ctx, request.ID); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
