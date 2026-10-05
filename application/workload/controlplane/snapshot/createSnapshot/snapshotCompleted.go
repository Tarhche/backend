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

// SnapshotCompleted writes down that a snapshot is stored and can be restored.
//
// A snapshot somebody deleted while it was being taken is taken away now that
// there is something to take away, and one nobody has a record of any more
// has its archive taken away too, rather than left in the bucket for ever.
type SnapshotCompleted struct {
	snapshotRepository snapshot.Repository
	remover            *archive.Remover
	logger             *slog.Logger
}

var _ domain.MessageHandler = &SnapshotCompleted{}

func NewSnapshotCompleted(snapshotRepository snapshot.Repository, remover *archive.Remover, logger *slog.Logger) *SnapshotCompleted {
	return &SnapshotCompleted{snapshotRepository: snapshotRepository, remover: remover, logger: logger}
}

// Handle never fails for a message that will never be handled: redelivering it
// would only fail the same way, at once and for ever.
func (h *SnapshotCompleted) Handle(ctx context.Context, data []byte) error {
	var completed events.SnapshotCompleted
	if err := json.Unmarshal(data, &completed); err != nil {
		h.logger.ErrorContext(ctx, "a snapshot completion that cannot be read", "error", err)

		return nil
	}

	s, err := h.snapshotRepository.GetOne(ctx, completed.SnapshotUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return h.remover.Remove(ctx, completed.SnapshotUUID)
	} else if err != nil {
		return err
	}

	switch s.State {
	case snapshot.Deleting:
		if err := h.remover.Remove(ctx, s.UUID); err != nil {
			return err
		}

		return h.snapshotRepository.Delete(ctx, s.UUID)

	case snapshot.Creating:
		s.State = snapshot.Ready
		s.Size = completed.Size
		s.Engine = completed.Engine
		s.Reason = ""

		if completed.Disk > 0 {
			s.Disk = completed.Disk
		}

		s.CompletedAt = completed.At
		if s.CompletedAt.IsZero() {
			s.CompletedAt = time.Now()
		}

		_, err := h.snapshotRepository.Save(ctx, &s)

		return err
	}

	return nil
}
