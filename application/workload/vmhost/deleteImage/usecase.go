// Package deleteImage lets go of an image's disk. An image a VM boots is
// kept for as long as the VM is held, whatever its tag says since: deleting
// one is refused with vm.ErrConflict.
package deleteImage

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase lets go of an image.
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

	if err := uc.engine.DeleteImage(ctx, request.Digest); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
