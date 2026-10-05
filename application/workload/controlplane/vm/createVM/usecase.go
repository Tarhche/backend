// Package createVM makes a VM: it checks what is asked for, holds it to what
// one VM and one person may be given, gives it a slug, chooses its node and
// asks that node to make it.
package createVM

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/quota"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// saveAttempts is how many slugs a VM is given before giving up, for the rare
// one taken between being found free and being written.
const saveAttempts = 3

// Images are what VMs boot from when they do not say.
type Images struct {
	// Machine is a machine VM's when it names none.
	Machine string

	// Docker is every Docker VM's: a docker-in-docker image, the same one the
	// nodes are given.
	Docker string
}

type UseCase struct {
	vmRepository       vm.Repository
	taskRepository     task.Repository
	snapshotRepository snapshot.Repository
	quota              *quota.Quota
	lifecycle          *lifecycle.Lifecycle
	validator          domain.Validator
	images             Images

	now func() time.Time
}

func NewUseCase(
	vmRepository vm.Repository,
	taskRepository task.Repository,
	snapshotRepository snapshot.Repository,
	quota *quota.Quota,
	lifecycle *lifecycle.Lifecycle,
	validator domain.Validator,
	images Images,
) *UseCase {
	return &UseCase{
		vmRepository:       vmRepository,
		taskRepository:     taskRepository,
		snapshotRepository: snapshotRepository,
		quota:              quota,
		lifecycle:          lifecycle,
		validator:          validator,
		images:             images,
		now:                time.Now,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, validationErrors, err := uc.vm(ctx, request)
	if err != nil {
		return nil, err
	}

	if len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	held, err := uc.quota.Check(ctx, "", v.OwnerUUID, v.Resources, "")
	if err != nil {
		return nil, err
	}

	if len(held) > 0 {
		return &Response{ValidationErrors: held}, nil
	}

	if err := uc.save(ctx, &v); err != nil {
		return nil, err
	}

	// placed, and its node asked to make it. With no node that has room, the
	// VM is failed rather than asked for again until one does: it is never left
	// for the messaging to retry, and whoever asked for it decides, by starting
	// it once there is room or by deleting it.
	if err := uc.lifecycle.Up(ctx, &v); err != nil && !errors.Is(err, vm.ErrNoCapacity) {
		return nil, err
	}

	shown := presenter.NewVM(&v)

	return &Response{VM: &shown}, nil
}

// vm is the VM the request asks for, with what it left out filled in, or why
// it cannot be had.
func (uc *UseCase) vm(ctx context.Context, request *Request) (vm.VM, domain.ValidationErrors, error) {
	now := uc.now()

	v := vm.VM{
		Name:           strings.TrimSpace(request.Name),
		OwnerUUID:      request.OwnerUUID,
		Kind:           request.Kind,
		Image:          request.Image,
		Resources:      request.Resources.VM(),
		Ports:          normalized(request.Ports),
		Network:        request.Network.VM(),
		PersistentDisk: request.PersistentDisk,
		Lifetime:       time.Duration(request.LifetimeSeconds) * time.Second,
		CurrentState:   vm.Created,
		ExpectedState:  vm.Running,
		UpdatedAt:      now,
	}

	validationErrors := make(domain.ValidationErrors)

	if len(request.SnapshotUUID) > 0 {
		if refused, err := uc.fromSnapshot(ctx, &v, request.SnapshotUUID); err != nil {
			return vm.VM{}, nil, err
		} else if len(refused) > 0 {
			return vm.VM{}, refused, nil
		}
	}

	switch v.Kind {
	case vm.KindDocker:
		// dockerd and the way it is started are the Docker image's, so a Docker
		// VM boots from nothing else. One restored from a snapshot boots from
		// what the snapshot holds.
		if len(request.SnapshotUUID) == 0 {
			if len(v.Image) > 0 && v.Image != uc.images.Docker {
				validationErrors["image"] = "invalid_image"
			}

			v.Image = uc.images.Docker
		}
	default:
		if len(v.Image) == 0 {
			v.Image = uc.images.Machine
		}
	}

	limits := uc.quota.Limits()
	merge(validationErrors, limits.Bounds("", v.Kind, v.Resources))
	merge(validationErrors, limits.Lifetime("", v.Lifetime))

	if v.Lifetime > 0 {
		v.ExpiresAt = now.Add(v.Lifetime)
	}

	return v, validationErrors, nil
}

// fromSnapshot makes v from a snapshot: its kind and image are the snapshot's,
// and its disk is at least the snapshot's.
func (uc *UseCase) fromSnapshot(ctx context.Context, v *vm.VM, snapshotUUID string) (domain.ValidationErrors, error) {
	s, err := uc.snapshotRepository.GetOneByOwner(ctx, v.OwnerUUID, snapshotUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return domain.ValidationErrors{"snapshot_uuid": "not_found"}, nil
	} else if err != nil {
		return nil, err
	}

	if s.State != snapshot.Ready {
		return domain.ValidationErrors{"snapshot_uuid": "snapshot_not_ready"}, nil
	}

	if len(v.Kind) > 0 && v.Kind != s.Kind {
		return domain.ValidationErrors{"kind": "kind_mismatch"}, nil
	}

	v.Kind = s.Kind
	v.Image = s.Image
	v.Resources.Disk = max(v.Resources.Disk, s.Disk)

	// the restore that makes it is pending until the VM first runs.
	v.RestoreFrom = s.UUID

	return nil, nil
}

// save gives the VM a slug nothing else holds, and writes it down. A slug taken
// between being found free and being written is answered with another one.
func (uc *UseCase) save(ctx context.Context, v *vm.VM) error {
	for range saveAttempts {
		generated, err := slugs.Generate(ctx, v.Name,
			slugs.By(uc.vmRepository.GetOneBySlug),
			slugs.By(uc.taskRepository.GetOneBySlug),
		)
		if err != nil {
			return err
		}

		v.Slug = generated

		_, err = uc.vmRepository.Save(ctx, v)
		if !errors.Is(err, domain.ErrAlreadyExists) {
			return err
		}
	}

	return slugs.ErrExhausted
}

// normalized is ports sorted, each once.
func normalized(ports []port.Port) []port.Port {
	sorted := slices.Clone(ports)
	slices.Sort(sorted)

	return slices.Compact(sorted)
}

func merge(into domain.ValidationErrors, from domain.ValidationErrors) {
	for field, code := range from {
		into[field] = code
	}
}
