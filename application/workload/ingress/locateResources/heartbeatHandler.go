package locateResources

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// HeartbeatHandler writes down where each instance a heartbeat says is.
type HeartbeatHandler struct {
	locations ingress.Locations
	kinds     map[string]Kind
	logger    *slog.Logger
}

var _ domain.MessageHandler = &HeartbeatHandler{}

// NewHeartbeatHandler is a handler that writes down in locations where each
// heartbeat says an instance of one of kinds, by name, is.
func NewHeartbeatHandler(locations ingress.Locations, kinds map[string]Kind, logger *slog.Logger) *HeartbeatHandler {
	return &HeartbeatHandler{locations: locations, kinds: kinds, logger: logger}
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

	// a kind the ingress does not route to is not listened to, and neither
	// is an instance nobody keeps a record of, which nothing names.
	k, routed := h.kinds[heartbeat.Kind]
	if !routed || len(heartbeat.UUID) == 0 {
		return nil
	}

	heard, err := k.Read(heartbeat.Status)
	if err != nil {
		h.logger.ErrorContext(ctx, "a heartbeat whose status cannot be read", "error", err, "kind", heartbeat.Kind, "uuid", heartbeat.UUID)

		return nil
	}

	heard.Kind = heartbeat.Kind
	heard.UUID = heartbeat.UUID
	heard.Node = heartbeat.Node

	heard.At = heartbeat.At
	if heard.At.IsZero() {
		heard.At = time.Now()
	}

	h.locations.Hear(ctx, heard)

	return nil
}
