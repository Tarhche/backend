package locateResources

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// ActOnResourceHandler takes away at once what each command sent to the
// node holding a resource takes away of it once it is carried out, before it
// is: every node hears every command, and so does every ingress.
type ActOnResourceHandler struct {
	locations ingress.Locations
	kinds     map[string]Kind
	logger    *slog.Logger
}

var _ domain.MessageHandler = &ActOnResourceHandler{}

// NewActOnResourceHandler is a handler that withholds in locations what each
// command to a resource of one of kinds, by name, takes away of it.
func NewActOnResourceHandler(locations ingress.Locations, kinds map[string]Kind, logger *slog.Logger) *ActOnResourceHandler {
	return &ActOnResourceHandler{locations: locations, kinds: kinds, logger: logger}
}

func (h *ActOnResourceHandler) Handle(ctx context.Context, data []byte) error {
	var command kind.ActOnResource
	if err := json.Unmarshal(data, &command); err != nil {
		// read again, it is as unreadable.
		h.logger.ErrorContext(ctx, "a command that cannot be read", "error", err)

		return nil
	}

	// a kind the ingress does not route to is not listened to, and neither
	// is a command about nothing in particular.
	k, routed := h.kinds[command.Kind]
	if !routed || len(command.UUID) == 0 {
		return nil
	}

	// an action its kind does not have is refused by its node, which leaves
	// the resource as it was.
	action, known := k.Descriptor().Action(command.Action)
	if !known {
		return nil
	}

	withheld := ingress.Withheld{Kind: command.Kind, UUID: command.UUID, Command: command.ID}

	// a command that leaves its resource anything but running, a stop or a
	// delete, takes it out of being reached, as its kind's descriptor says
	// of what its action desires.
	if len(action.Desires) > 0 && action.Desires != k.Running() {
		withheld.State = action.Desires
	}

	// and none leaves it reached on more ports than it lets in as the
	// command carries it. A resource that cannot be read lets nothing in: a
	// command only ever takes away.
	allowed, err := k.Allowed(command.Resource)
	if err != nil {
		h.logger.ErrorContext(ctx, "a command whose resource cannot be read", "error", err, "kind", command.Kind, "uuid", command.UUID, "action", command.Action)
	}

	withheld.Ports = allowed

	h.locations.Withhold(ctx, withheld)

	return nil
}
