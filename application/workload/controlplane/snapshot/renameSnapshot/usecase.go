// Package renameSnapshot gives a snapshot another name.
package renameSnapshot

import (
	"context"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

type UseCase struct {
	snapshotRepository snapshot.Repository
	validator          domain.Validator
}

func NewUseCase(snapshotRepository snapshot.Repository, validator domain.Validator) *UseCase {
	return &UseCase{snapshotRepository: snapshotRepository, validator: validator}
}

// Execute renames the snapshot. One still being taken is not renamed: its node
// is about to say how it went, and that would be written over the new name, or
// the new name over it. A snapshot that is not there, or not the owner's, is
// domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	s, err := owner.One(ctx, uc.snapshotRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	if s.State != snapshot.Ready && s.State != snapshot.Failed {
		return &Response{ValidationErrors: domain.ValidationErrors{"snapshot": "invalid_state_transition"}}, nil
	}

	s.Name = strings.TrimSpace(request.Name)

	if _, err := uc.snapshotRepository.Save(ctx, &s); err != nil {
		return nil, err
	}

	shown := presenter.NewSnapshot(&s)

	return &Response{Snapshot: &shown}, nil
}
