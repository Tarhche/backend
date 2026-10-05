// Package deleteSnapshot removes a snapshot: its archive, which the control
// plane takes away from the bucket itself, and then its record.
package deleteSnapshot

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/archive"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	snapshotRepository snapshot.Repository
	vmRepository       vm.Repository
	lifecycle          *lifecycle.Lifecycle
	remover            *archive.Remover
	validator          domain.Validator
}

func NewUseCase(
	snapshotRepository snapshot.Repository,
	vmRepository vm.Repository,
	lifecycle *lifecycle.Lifecycle,
	remover *archive.Remover,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		snapshotRepository: snapshotRepository,
		vmRepository:       vmRepository,
		lifecycle:          lifecycle,
		remover:            remover,
		validator:          validator,
	}
}

// Execute removes the snapshot. One still being taken goes once its node has
// finished with it, so that an archive written after it was deleted is not left
// behind; one whose node is gone, and so will never finish, goes at once. A
// snapshot that is not there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	s, err := owner.One(ctx, uc.snapshotRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	if s.State == snapshot.Creating {
		taking, err := uc.beingTaken(ctx, &s)
		if err != nil {
			return nil, err
		}

		if taking {
			s.State = snapshot.Deleting
			if _, err := uc.snapshotRepository.Save(ctx, &s); err != nil {
				return nil, err
			}

			return &Response{Pending: true}, nil
		}
	}

	// written down first, so that one whose archive could not be taken away
	// is shown as going rather than as kept.
	if s.State != snapshot.Deleting {
		s.State = snapshot.Deleting
		if _, err := uc.snapshotRepository.Save(ctx, &s); err != nil {
			return nil, err
		}
	}

	if err := uc.remover.Remove(ctx, s.UUID); err != nil {
		return nil, err
	}

	return &Response{}, uc.snapshotRepository.Delete(ctx, s.UUID)
}

// beingTaken reports whether the node a snapshot is being taken on is still
// there to finish it.
func (uc *UseCase) beingTaken(ctx context.Context, s *snapshot.Snapshot) (bool, error) {
	v, err := uc.vmRepository.GetOne(ctx, s.VMUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return uc.lifecycle.NodeAlive(ctx, v.NodeName)
}
