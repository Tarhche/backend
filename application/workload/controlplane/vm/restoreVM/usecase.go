// Package restoreVM replaces a VM's disk from a snapshot. The VM keeps its
// uuid, its slug and its ports; only what is on its disk changes.
package restoreVM

import (
	"context"
	"errors"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/command"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	vmRepository       vm.Repository
	runs               *coderunner.Runs
	snapshotRepository snapshot.Repository
	nodeRepository     node.Repository
	lifecycle          *lifecycle.Lifecycle
	commander          *command.Commander
	validator          domain.Validator
}

func NewUseCase(
	vmRepository vm.Repository,
	runs *coderunner.Runs,
	snapshotRepository snapshot.Repository,
	nodeRepository node.Repository,
	lifecycle *lifecycle.Lifecycle,
	commander *command.Commander,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		vmRepository:       vmRepository,
		runs:               runs,
		snapshotRepository: snapshotRepository,
		nodeRepository:     nodeRepository,
		lifecycle:          lifecycle,
		commander:          commander,
		validator:          validator,
	}
}

// Execute asks the VM's node to restore it, or says why it cannot be. A VM
// that is not there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		// a uuid that names no VM may name a run, whose disk is the code
		// runner's.
		refused, err := uc.runs.Refused(ctx, request.OwnerUUID, request.UUID, err)
		if err != nil {
			return nil, err
		}

		return &Response{ValidationErrors: refused}, nil
	}

	s, refused, err := uc.snapshot(ctx, &v, request.SnapshotUUID)
	if err != nil {
		return nil, err
	}

	if len(refused) > 0 {
		return &Response{ValidationErrors: refused}, nil
	}

	alive, err := uc.lifecycle.NodeAlive(ctx, v.NodeName)
	if err != nil {
		return nil, err
	}

	// the disk is replaced where it is, so the VM has to be on a node that can
	// be asked, and resting: one on its way somewhere has been asked for
	// something already.
	if !alive || !vm.ValidStateTransition(v.CurrentState, vm.Restoring) {
		return &Response{ValidationErrors: domain.ValidationErrors{"vm": "invalid_state_transition"}}, nil
	}

	if refused := uc.engine(ctx, &v, &s); len(refused) > 0 {
		return &Response{ValidationErrors: refused}, nil
	}

	// whoever restores a VM they had given up on wants it back.
	if v.ExpectedState != vm.Stopped {
		v.ExpectedState = vm.Running
	}

	v.CurrentState = vm.Restoring
	v.RestoreFrom = s.UUID
	v.Reason = ""
	v.UpdatedAt = uc.lifecycle.Now()

	if _, err := uc.vmRepository.Save(ctx, &v); err != nil {
		return nil, err
	}

	if err := uc.commander.Restore(ctx, &v, s.UUID); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

// snapshot is the snapshot to restore from: stored, the VM owner's, of the
// VM's kind, and with a disk no larger than the VM's.
func (uc *UseCase) snapshot(ctx context.Context, v *vm.VM, uuid string) (snapshot.Snapshot, domain.ValidationErrors, error) {
	s, err := uc.snapshotRepository.GetOneByOwner(ctx, v.OwnerUUID, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return snapshot.Snapshot{}, domain.ValidationErrors{"snapshot_uuid": "not_found"}, nil
	} else if err != nil {
		return snapshot.Snapshot{}, nil, err
	}

	switch {
	case s.State != snapshot.Ready:
		return s, domain.ValidationErrors{"snapshot_uuid": "snapshot_not_ready"}, nil
	case s.Kind != v.Kind:
		return s, domain.ValidationErrors{"snapshot_uuid": "kind_mismatch"}, nil
	case s.Disk > v.Resources.Disk:
		return s, domain.ValidationErrors{"snapshot_uuid": "disk_too_small"}, nil
	}

	return s, nil, nil
}

// engine refuses a snapshot another engine wrote, when the VM's node has said
// which engine it runs. Whether a version of the same engine can read it is
// the engine's to say, which it does by failing the restore.
func (uc *UseCase) engine(ctx context.Context, v *vm.VM, s *snapshot.Snapshot) domain.ValidationErrors {
	n, err := uc.nodeRepository.GetOne(ctx, v.NodeName)
	if err != nil || len(n.Capacity.Engine) == 0 || len(s.Engine) == 0 {
		return nil
	}

	written, _, _ := strings.Cut(s.Engine, "/")
	if written != n.Capacity.Engine {
		return domain.ValidationErrors{"snapshot_uuid": "engine_mismatch"}
	}

	return nil
}
