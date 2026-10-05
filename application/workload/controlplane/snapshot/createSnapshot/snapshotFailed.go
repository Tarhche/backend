package createSnapshot

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/archive"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
)

// defaultReason is said of a snapshot whose node did not say what went wrong.
const defaultReason = "the snapshot could not be taken"

// SnapshotFailed writes down that a snapshot could not be taken or stored. A
// snapshot is never taken again: one that failed is deleted, and another is
// asked for. One somebody deleted while it was being taken goes now.
type SnapshotFailed struct {
	snapshotRepository snapshot.Repository
	remover            *archive.Remover
	logger             *slog.Logger
}

var _ domain.MessageHandler = &SnapshotFailed{}

func NewSnapshotFailed(snapshotRepository snapshot.Repository, remover *archive.Remover, logger *slog.Logger) *SnapshotFailed {
	return &SnapshotFailed{snapshotRepository: snapshotRepository, remover: remover, logger: logger}
}

// Handle never fails for a message that will never be handled: redelivering it
// would only fail the same way, at once and for ever.
func (h *SnapshotFailed) Handle(ctx context.Context, data []byte) error {
	var failed events.SnapshotFailed
	if err := json.Unmarshal(data, &failed); err != nil {
		h.logger.ErrorContext(ctx, "a snapshot failure that cannot be read", "error", err)

		return nil
	}

	s, err := h.snapshotRepository.GetOne(ctx, failed.SnapshotUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	h.logger.WarnContext(ctx, "a snapshot failed", "uuid", s.UUID, "vm", s.VMUUID, "node", failed.NodeName, "reason", failed.Reason)

	switch s.State {
	case snapshot.Deleting:
		// nothing should be left of it, and whatever is goes with it.
		if err := h.remover.Remove(ctx, s.UUID); err != nil {
			return err
		}

		return h.snapshotRepository.Delete(ctx, s.UUID)

	case snapshot.Creating:
		s.State = snapshot.Failed
		s.Reason = failed.Reason
		if len(s.Reason) == 0 {
			s.Reason = defaultReason
		}

		s.CompletedAt = failed.At
		if s.CompletedAt.IsZero() {
			s.CompletedAt = time.Now()
		}

		_, err := h.snapshotRepository.Save(ctx, &s)

		return err
	}

	return nil
}
