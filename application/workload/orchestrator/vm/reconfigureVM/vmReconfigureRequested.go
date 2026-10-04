package reconfigureVM

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// VMReconfigureRequestedHandler applies to the VMs of this node the changes
// the control plane asks for.
type VMReconfigureRequestedHandler struct {
	useCase  *UseCase
	producer domain.Producer
	nodeName string
	logger   *slog.Logger
}

var _ domain.MessageHandler = &VMReconfigureRequestedHandler{}

func NewVMReconfigureRequestedHandler(useCase *UseCase, producer domain.Producer, nodeName string, logger *slog.Logger) *VMReconfigureRequestedHandler {
	return &VMReconfigureRequestedHandler{useCase: useCase, producer: producer, nodeName: nodeName, logger: logger}
}

func (h *VMReconfigureRequestedHandler) Handle(ctx context.Context, data []byte) error {
	var requested events.VMReconfigureRequested
	if err := json.Unmarshal(data, &requested); err != nil {
		h.logger.ErrorContext(ctx, "a vm could not be reconfigured from an unreadable message", "error", err)

		return nil
	}

	if requested.NodeName != h.nodeName {
		return nil
	}

	response, err := h.useCase.Execute(ctx, &Request{VMUUID: requested.VMUUID, Spec: requested.Spec.ToVM()})
	if err != nil {
		return err
	}

	return vmcommand.Refused(ctx, h.producer, h.nodeName, requested.VMUUID, response.ValidationErrors)
}
