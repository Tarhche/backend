// Package archive takes a snapshot's archive away from the snapshots bucket.
//
// The control plane does it itself, rather than asking a node, because it holds
// the bucket's settings and a snapshot outlives the VM and the node it was
// taken on.
package archive

import (
	"context"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

// Remover takes archives away.
type Remover struct {
	// store is nil when the control plane was given no bucket, in which case
	// there is nowhere for it to remove an archive from.
	store  snapshot.Store
	logger *slog.Logger
}

func NewRemover(store snapshot.Store, logger *slog.Logger) *Remover {
	return &Remover{store: store, logger: logger}
}

// Remove takes a snapshot's archive away. One that is not there is the outcome
// asked for: a snapshot that failed left nothing behind, and one deleted twice
// has nothing left the second time.
func (r *Remover) Remove(ctx context.Context, uuid string) error {
	if r.store == nil {
		r.logger.WarnContext(ctx, "no snapshots bucket is configured, so a snapshot's archive is left where it is", "snapshot", uuid)

		return nil
	}

	err := r.store.Delete(ctx, snapshot.ObjectKey(uuid))
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return err
}
