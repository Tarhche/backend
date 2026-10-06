// Package createSnapshot asks the node holding a VM to take a snapshot of its
// disk and store it, and hears when it has, or could not.
package createSnapshot

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Runs say whether a uuid that names no VM names one of the code runner's
// runs, whose disk is thrown away with it, and refuse it if so.
type Runs interface {
	Refused(ctx context.Context, ownerUUID string, uuid string, err error) (domain.ValidationErrors, error)
}

// Nodes say whether a node can be asked anything, having spoken lately.
type Nodes interface {
	Alive(ctx context.Context, nodeName string) (bool, error)
}

type UseCase struct {
	vmRepository       owner.Repository[vm.VM]
	runs               Runs
	snapshotRepository snapshot.Repository
	nodes              Nodes
	producer           domain.Producer
	validator          domain.Validator

	// userMax is the most snapshots one person may keep.
	userMax uint
}

func NewUseCase(
	vmRepository owner.Repository[vm.VM],
	runs Runs,
	snapshotRepository snapshot.Repository,
	nodes Nodes,
	producer domain.Producer,
	validator domain.Validator,
	userMax uint,
) *UseCase {
	return &UseCase{
		vmRepository:       vmRepository,
		runs:               runs,
		snapshotRepository: snapshotRepository,
		nodes:              nodes,
		producer:           producer,
		validator:          validator,
		userMax:            userMax,
	}
}

// Execute asks for the snapshot, or says why it cannot be taken. A VM that is
// not there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.VMUUID)
	if err != nil {
		// a uuid that names no VM may name a run, whose disk is thrown away
		// with it.
		refused, err := uc.runs.Refused(ctx, request.OwnerUUID, request.VMUUID, err)
		if err != nil {
			return nil, err
		}

		return &Response{ValidationErrors: refused}, nil
	}

	alive, err := uc.nodes.Alive(ctx, v.NodeName)
	if err != nil {
		return nil, err
	}

	// a disk is taken from a VM that is resting or running, on a node that can
	// be asked: one on its way somewhere is changing under it.
	if !alive || (v.CurrentState != vm.Running && v.CurrentState != vm.Stopped) {
		return &Response{ValidationErrors: domain.ValidationErrors{"vm": "invalid_state_transition"}}, nil
	}

	kept, err := uc.snapshotRepository.CountByOwner(ctx, v.OwnerUUID)
	if err != nil {
		return nil, err
	}

	if kept+1 > uc.userMax {
		return &Response{ValidationErrors: domain.ValidationErrors{"snapshots": "quota_exceeded"}}, nil
	}

	s := snapshot.Snapshot{
		Name:      strings.TrimSpace(request.Name),
		OwnerUUID: v.OwnerUUID,
		VMUUID:    v.UUID,
		VMName:    v.Name,
		Kind:      v.Kind,
		Image:     v.Image,
		Disk:      v.Resources.Disk,
		State:     snapshot.Creating,
	}

	if _, err := uc.snapshotRepository.Save(ctx, &s); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(events.SnapshotRequested{SnapshotUUID: s.UUID, VMUUID: v.UUID, NodeName: v.NodeName})
	if err != nil {
		return nil, err
	}

	if err := uc.producer.Produce(context.WithoutCancel(ctx), events.SnapshotRequestedName, payload); err != nil {
		return nil, err
	}

	shown := presenter.NewSnapshot(&s)

	return &Response{Snapshot: &shown}, nil
}
