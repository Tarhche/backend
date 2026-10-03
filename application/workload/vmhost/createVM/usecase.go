// Package createVM makes a VM: its image made ready, its record, and its
// scratch disk. It boots nothing, as docker's create starts nothing.
//
// A VM is refused for want of room here, with vm.ErrCapacity, while the task
// can still be placed on another node; one that asks for more than one VM may
// have here is invalid, since no amount of waiting makes room for it; and one
// whose name another VM answers to is a conflict, which the orchestrator takes
// for the same task asked for twice.
package createVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// Limits are the most one VM may be given on this host.
type Limits struct {
	// MaxMemory is in bytes, and MaxCPU in CPUs; zero is no limit.
	MaxMemory uint64
	MaxCPU    float64
}

// UseCase makes VMs.
type UseCase struct {
	engine    *vmhost.Engine
	validator domain.Validator
	limits    Limits
}

func NewUseCase(engine *vmhost.Engine, validator domain.Validator, limits Limits) *UseCase {
	return &UseCase{engine: engine, validator: validator, limits: limits}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	// what one VM may have is the host's to say, so it is checked here
	// rather than with the rest of what was asked.
	if validationErrors := uc.withinLimits(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	id, err := uc.engine.Create(ctx, request.Spec)
	if err != nil {
		return nil, err
	}

	return &Response{ID: id}, nil
}

func (uc *UseCase) withinLimits(request *Request) domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if uc.limits.MaxMemory > 0 && request.Resources.Memory > uc.limits.MaxMemory {
		validationErrors["resources.memory"] = "exceeds_limit"
	}

	if uc.limits.MaxCPU > 0 && request.Resources.CPU > uc.limits.MaxCPU {
		validationErrors["resources.cpu"] = "exceeds_limit"
	}

	return validationErrors
}
