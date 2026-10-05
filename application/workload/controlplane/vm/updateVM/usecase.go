// Package updateVM changes a VM: its name and lifetime, which are the record's
// alone, and its ports, network and resources, which its node applies to it,
// restarting it when the engine has to.
package updateVM

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/quota"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	vmRepository vm.Repository
	quota        *quota.Quota
	placement    *placement.Placement
	lifecycle    *lifecycle.Lifecycle
	validator    domain.Validator
}

func NewUseCase(
	vmRepository vm.Repository,
	quota *quota.Quota,
	placement *placement.Placement,
	lifecycle *lifecycle.Lifecycle,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		vmRepository: vmRepository,
		quota:        quota,
		placement:    placement,
		lifecycle:    lifecycle,
		validator:    validator,
	}
}

// Execute changes the VM, or says why it cannot. A VM that is not there, or not
// the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	if v.CurrentState == vm.Deleting {
		return &Response{ValidationErrors: domain.ValidationErrors{"vm": "invalid_state_transition"}}, nil
	}

	now := uc.lifecycle.Now()
	limits := uc.quota.Limits()
	validationErrors := make(domain.ValidationErrors)

	if request.Name != nil {
		v.Name = strings.TrimSpace(*request.Name)
	}

	if request.LifetimeSeconds != nil {
		v.Lifetime = time.Duration(*request.LifetimeSeconds) * time.Second
		v.ExpiresAt = time.Time{}

		if v.Lifetime > 0 {
			v.ExpiresAt = now.Add(v.Lifetime)
		}

		merge(validationErrors, limits.Lifetime("", v.Lifetime))
	}

	changed, err := uc.respec(ctx, &v, request, validationErrors)
	if err != nil {
		return nil, err
	}

	if len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	if changed {
		err = uc.lifecycle.Reconfigure(ctx, &v)
	} else {
		v.UpdatedAt = now
		_, err = uc.vmRepository.Save(ctx, &v)
	}

	if err != nil {
		return nil, err
	}

	shown := presenter.NewVM(&v)

	return &Response{VM: &shown}, nil
}

// respec gives v the ports, network and resources the request asks for, and
// reports whether they are any different from what it had. What cannot be
// given is written into validationErrors.
func (uc *UseCase) respec(ctx context.Context, v *vm.VM, request *Request, validationErrors domain.ValidationErrors) (bool, error) {
	ports, network, resources := v.Ports, v.Network, v.Resources

	if request.Ports != nil {
		ports = slices.Compact(slices.Sorted(slices.Values(*request.Ports)))
	}

	if request.Network != nil {
		if len(request.Network.Ingress) > 0 {
			network.Ingress = request.Network.Ingress
		}

		if len(request.Network.Egress) > 0 {
			network.Egress = request.Network.Egress
		}
	}

	if request.Resources != nil {
		resources = request.Resources.VM()
	}

	changed := !slices.Equal(ports, v.Ports) || network != v.Network || resources != v.Resources
	if !changed {
		return false, nil
	}

	// a VM on its way somewhere has been asked for something already, and a
	// change sent after it could reach its node before it.
	if vm.IsInFlightState(v.CurrentState) {
		validationErrors["vm"] = "invalid_state_transition"

		return true, nil
	}

	if resources != v.Resources {
		refused, err := uc.resources(ctx, v, resources)
		if err != nil {
			return false, err
		}

		merge(validationErrors, refused)
	}

	v.Ports = ports
	if v.Ports == nil {
		v.Ports = []port.Port{}
	}

	v.Network = network
	v.Resources = resources

	return true, nil
}

// resources checks what a VM would be given instead of what it has: what one
// VM may be given, what its owner's VMs may be given between them, and what its
// node has room for. Its disk only grows: a disk is not taken back from under
// what is written on it.
func (uc *UseCase) resources(ctx context.Context, v *vm.VM, resources vm.Resources) (domain.ValidationErrors, error) {
	refused := uc.quota.Limits().Bounds("", v.Kind, resources)

	if resources.Disk < v.Resources.Disk {
		refused["resources.disk"] = quota.CodeTooSmall
	}

	if len(refused) > 0 {
		return refused, nil
	}

	held, err := uc.quota.Check(ctx, "", v.OwnerUUID, resources, v.UUID)
	if err != nil {
		return nil, err
	}

	if len(held) > 0 {
		return held, nil
	}

	fits, err := uc.placement.Fits(ctx, v, resources)
	if err != nil {
		return nil, err
	}

	if !fits {
		return domain.ValidationErrors{"resources": lifecycle.ReasonNoCapacity}, nil
	}

	return nil, nil
}

func merge(into domain.ValidationErrors, from domain.ValidationErrors) {
	for field, code := range from {
		into[field] = code
	}
}
