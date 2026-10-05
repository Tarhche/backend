package restartVM

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// VMRestartRequestedHandler restarts the VMs of this node the control plane
// asks to be restarted.
type VMRestartRequestedHandler struct {
	useCase  *UseCase
	producer domain.Producer
	nodeName string
	logger   *slog.Logger
}

var _ domain.MessageHandler = &VMRestartRequestedHandler{}

func NewVMRestartRequestedHandler(useCase *UseCase, producer domain.Producer, nodeName string, logger *slog.Logger) *VMRestartRequestedHandler {
	return &VMRestartRequestedHandler{useCase: useCase, producer: producer, nodeName: nodeName, logger: logger}
}

func (h *VMRestartRequestedHandler) Handle(ctx context.Context, data []byte) error {
	var requested events.VMRestartRequested
	if err := json.Unmarshal(data, &requested); err != nil {
		h.logger.ErrorContext(ctx, "a vm could not be restarted from an unreadable message", "error", err)

		return nil
	}

	if requested.NodeName != h.nodeName {
		return nil
	}

	response, err := h.useCase.Execute(ctx, &Request{VMUUID: requested.VMUUID})
	if err != nil {
		return err
	}

	return vmcommand.Refused(ctx, h.producer, h.nodeName, requested.VMUUID, response.ValidationErrors)
}
