// Package heartbeatResources hears what the nodes hold of every kind whose
// state is theirs to say, in a heartbeat of each kind's own, on the kind's own
// subject (kind.HeartbeatName), and hands it to whoever writes it down.
//
// A heartbeat says everything of one kind its node holds, so what it leaves
// out is gone from the node; a kind that sent none could not look, and
// nothing is concluded from its silence. What a node says of itself, that it
// is alive and what it offers, is heard in its own heartbeat
// (node/heartbeatNode), and the heartbeats of one beat, the node's and its
// kinds', are heard in no particular order.
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

// Observer writes down what a node's heartbeat of one kind at a moment says
// of the resources of that kind the node holds. It fails nothing: the next
// beat says it all again.
type Observer interface {
	Heartbeat(ctx context.Context, nodeName string, kindName string, at time.Time, report kind.Report[json.RawMessage])
}

// HeartbeatHandler hands what a kind's heartbeat says to the observer.
type HeartbeatHandler struct {
	observer Observer
	logger   *slog.Logger
}

var _ domain.MessageHandler = &HeartbeatHandler{}

// NewHeartbeatHandler is a handler that hands what each kind's heartbeat says
// of the resources its node holds to observer.
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

	// one that names no node would be taken to speak for what every node
	// holds, and one that names no kind speaks for none.
	if len(heartbeat.Node) == 0 || len(heartbeat.Kind) == 0 {
		h.logger.ErrorContext(ctx, "a heartbeat that names no node or no kind", "node", heartbeat.Node, "kind", heartbeat.Kind)

		return nil
	}

	at := heartbeat.At
	if at.IsZero() {
		at = time.Now()
	}

	h.observer.Heartbeat(ctx, heartbeat.Node, heartbeat.Kind, at, heartbeat.Report)

	return nil
}
