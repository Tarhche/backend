// Package getInfo says what vmhost is: its version and its hypervisor's,
// whether it can run VMs right now and why not, what the class it stands
// behind can do, and how much of its budget is taken. The orchestrator's
// microvm driver asks it on every node heartbeat and offers the class as it
// is told.
package getInfo

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
)

// UseCase says what vmhost is.
type UseCase struct {
	engine *vmhost.Engine
}

func NewUseCase(engine *vmhost.Engine) *UseCase {
	return &UseCase{engine: engine}
}

// Execute says what vmhost is. It reads nothing but memory: it is asked once
// a second.
func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	info, err := uc.engine.Info(ctx)
	if err != nil {
		return nil, err
	}

	return &Response{Info: info}, nil
}
