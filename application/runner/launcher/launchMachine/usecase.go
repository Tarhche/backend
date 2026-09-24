package launchMachine

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// Limits are the most a host lets one machine have.
type Limits struct {
	VCPUs     int
	MemoryMiB int
}

// UseCase starts a machine's process on this host and plugs it into its
// networks.
type UseCase struct {
	vmm       machine.VMM
	network   machine.HostNetwork
	validator domain.Validator
	limits    Limits
}

func NewUseCase(vmm machine.VMM, network machine.HostNetwork, validator domain.Validator, limits Limits) *UseCase {
	return &UseCase{
		vmm:       vmm,
		network:   network,
		validator: validator,
		limits:    limits,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	// what a host can spare is the host's to say, so it is checked here
	// rather than with the rest of what was asked.
	if validationErrors := uc.withinLimits(request.Spec); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	// a machine asked for again while it runs is the one that is running,
	// and its taps are in use: they are left as they are.
	running, found, err := uc.running(ctx, request.ID)
	if err != nil {
		return nil, err
	}

	if found {
		return &Response{Machine: running}, nil
	}

	launched, err := uc.vmm.Spawn(ctx, request.Spec)
	if err != nil {
		return nil, err
	}

	// the process opens its taps only once it is configured, which is after
	// this answers, so they are made now, for the user it runs as. A machine
	// is never handed over without the devices it was promised.
	taps, err := uc.network.Plug(ctx, request.Owner, request.ID, launched.User, request.Taps)
	if err != nil {
		return nil, errors.Join(err, uc.network.Unplug(ctx, request.ID), uc.vmm.Kill(ctx, request.ID))
	}

	launched.Taps = taps

	return &Response{Machine: launched}, nil
}

// running is the machine by that id, if its process is running.
func (uc *UseCase) running(ctx context.Context, id string) (machine.Machine, bool, error) {
	held, err := uc.vmm.List(ctx)
	if err != nil {
		return machine.Machine{}, false, err
	}

	for _, m := range held {
		if m.ID == id && m.Running {
			return m, true, nil
		}
	}

	return machine.Machine{}, false, nil
}

func (uc *UseCase) withinLimits(spec machine.Spec) domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if uc.limits.VCPUs > 0 && spec.VCPUs > uc.limits.VCPUs {
		validationErrors["vcpus"] = "too_many"
	}

	if uc.limits.MemoryMiB > 0 && spec.MemoryMiB > uc.limits.MemoryMiB {
		validationErrors["memory_mib"] = "too_many"
	}

	return validationErrors
}
