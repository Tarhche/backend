// Package getImages lists the images made into disks here, for whoever looks
// after vmhost's disk.
package getImages

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
)

// UseCase lists the images kept here.
type UseCase struct {
	engine *vmhost.Engine
}

func NewUseCase(engine *vmhost.Engine) *UseCase {
	return &UseCase{engine: engine}
}

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	images, err := uc.engine.Images(ctx)
	if err != nil {
		return nil, err
	}

	return &Response{Images: images}, nil
}
