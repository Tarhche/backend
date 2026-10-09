// Package heartbeatResources hears what the nodes hold of every kind whose
// state is theirs to say, an instance in each heartbeat, on its kind's own
// subject (kind.HeartbeatName), and hands it to whoever writes it down.
//
// A heartbeat says one instance its node holds, and nothing of what the node
// does not: a resource its node goes on beating without a word of for long
// enough is taken to be gone by the reconcile loop (reconcileResources), not
// here. What a node says of itself, that it is alive and what it offers, is
// heard in its own heartbeat (node/heartbeatNode), and the heartbeats of one
// beat, the node's and its instances', are heard in no particular order.
//
// A heartbeat is never failed: the next beat says it all again. One that
// cannot be read, or that names no node or no kind, is let go of.
package heartbeatResources

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Observer writes down what a node's heartbeat at a moment says of one
// instance it holds. It fails nothing: the next beat says it all again.
type Observer interface {
	Heartbeat(ctx context.Context, nodeName string, at time.Time, instance kind.Observation)
}

// HeartbeatHandler hands what an instance's heartbeat says to the observer.
type HeartbeatHandler struct {
	observer Observer
	logger   *slog.Logger
}

var _ domain.MessageHandler = &HeartbeatHandler{}

// NewHeartbeatHandler is a handler that hands what each heartbeat says of an
// instance its node holds to observer.
func NewHeartbeatHandler(observer Observer, logger *slog.Logger) *HeartbeatHandler {
	return &HeartbeatHandler{observer: observer, logger: logger}
}

func (h *HeartbeatHandler) Handle(ctx context.Context, data []byte) error {
	var heartbeat kind.Heartbeat
	if err := json.Unmarshal(data, &heartbeat); err != nil {
		// read again, it is as unreadable.
		h.logger.ErrorContext(ctx, "a heartbeat that cannot be read", "error", err)

		return nil
	}

	// one that names no node speaks for nothing a node holds, and one that
	// names no kind for nothing of any kind.
	if len(heartbeat.Node) == 0 || len(heartbeat.Kind) == 0 {
		h.logger.ErrorContext(ctx, "a heartbeat that names no node or no kind", "node", heartbeat.Node, "kind", heartbeat.Kind)

		return nil
	}

	at := heartbeat.At
	if at.IsZero() {
		at = time.Now()
	}

	h.observer.Heartbeat(ctx, heartbeat.Node, at, heartbeat.Observed)

	return nil
}
