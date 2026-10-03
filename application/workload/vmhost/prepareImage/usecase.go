// Package prepareImage makes an image ready to boot: pulled for the host's
// platform, made into a disk, and kept by its digest. The orchestrator asks for
// it before it makes a VM, so that pulling an image it has never seen is not
// counted against the time a task may run for. Asking twice is asking once.
package prepareImage

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase makes an image ready to boot.
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

	image, err := uc.engine.PrepareImage(ctx, request.Image)
	if err != nil {
		return nil, err
	}

	return &Response{Image: image}, nil
}
