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

// ResourceActedOnHandler writes down what came of each command sent to the
// node holding a resource: what the command withheld comes back, and where it
// left the resource, when it was carried out, is taken as a heartbeat is, at
// once, rather than at the node's next beat.
type ResourceActedOnHandler struct {
	locations ingress.Locations
	kinds     map[string]Kind
	logger    *slog.Logger
}

var _ domain.MessageHandler = &ResourceActedOnHandler{}

// NewResourceActedOnHandler is a handler that writes down in locations what
// came of each command to a resource of one of kinds, by name.
func NewResourceActedOnHandler(locations ingress.Locations, kinds map[string]Kind, logger *slog.Logger) *ResourceActedOnHandler {
	return &ResourceActedOnHandler{locations: locations, kinds: kinds, logger: logger}
}

func (h *ResourceActedOnHandler) Handle(ctx context.Context, data []byte) error {
	var result kind.ResourceActedOn
	if err := json.Unmarshal(data, &result); err != nil {
		// read again, it is as unreadable.
		h.logger.ErrorContext(ctx, "a command's result that cannot be read", "error", err)

		return nil
	}

	// a kind the ingress does not route to is not listened to, and neither
	// is a result about nothing in particular.
	k, routed := h.kinds[result.Kind]
	if !routed || len(result.UUID) == 0 {
		return nil
	}

	answered := ingress.Answered{Kind: result.Kind, UUID: result.UUID, Command: result.ID}

	// one that failed or was refused, or that says nothing of where it left
	// the resource, or of which node it was carried out on, changes nothing
	// but that its command is answered.
	if result.OK && !result.Refused && len(result.Status) > 0 && len(result.Node) > 0 {
		heard, err := h.heard(k, result)
		if err != nil {
			h.logger.ErrorContext(ctx, "a command's result whose status cannot be read", "error", err, "kind", result.Kind, "uuid", result.UUID, "action", result.Action)
		} else {
			answered.Heard = &heard
		}
	}

	h.locations.Answer(ctx, answered)

	return nil
}

// heard is where a command left its resource, as its result says: nowhere,
// once a command whose action desires it deleted has deleted it.
func (h *ResourceActedOnHandler) heard(k Kind, result kind.ResourceActedOn) (ingress.Heard, error) {
	var heard ingress.Heard

	if action, _ := k.Descriptor().Action(result.Action); action.Desires == kind.Deleted {
		heard.Gone = true
	} else {
		read, err := k.Read(result.Status)
		if err != nil {
			return ingress.Heard{}, err
		}

		heard = read
	}

	heard.Kind = result.Kind
	heard.UUID = result.UUID
	heard.Node = result.Node

	heard.At = result.At
	if heard.At.IsZero() {
		heard.At = time.Now()
	}

	return heard, nil
}
