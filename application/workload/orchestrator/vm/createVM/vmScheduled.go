package createVM

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// VMScheduledHandler creates the VMs the control plane schedules on this node.
type VMScheduledHandler struct {
	useCase  *UseCase
	producer domain.Producer
	nodeName string
	logger   *slog.Logger
}

var _ domain.MessageHandler = &VMScheduledHandler{}

func NewVMScheduledHandler(useCase *UseCase, producer domain.Producer, nodeName string, logger *slog.Logger) *VMScheduledHandler {
	return &VMScheduledHandler{useCase: useCase, producer: producer, nodeName: nodeName, logger: logger}
}

func (h *VMScheduledHandler) Handle(ctx context.Context, data []byte) error {
	var scheduled events.VMScheduled
	if err := json.Unmarshal(data, &scheduled); err != nil {
		// a message that cannot be read now never will be.
		h.logger.ErrorContext(ctx, "a vm could not be scheduled from an unreadable message", "error", err)

		return nil
	}

	// every node hears every command, and acts only on its own
	if scheduled.NodeName != h.nodeName {
		return nil
	}

	response, err := h.useCase.Execute(ctx, &Request{
		VMUUID:       scheduled.VMUUID,
		Spec:         scheduled.Spec.ToVM(),
		SnapshotUUID: scheduled.SnapshotUUID,
	})
	if err != nil {
		return err
	}

	return vmcommand.Refused(ctx, h.producer, h.nodeName, scheduled.VMUUID, response.ValidationErrors)
}
