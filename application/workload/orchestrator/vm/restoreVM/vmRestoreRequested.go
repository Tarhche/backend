package restoreVM

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// VMRestoreRequestedHandler restores the VMs of this node the control plane
// asks to be restored from a snapshot.
type VMRestoreRequestedHandler struct {
	useCase  *UseCase
	producer domain.Producer
	nodeName string
	logger   *slog.Logger
}

var _ domain.MessageHandler = &VMRestoreRequestedHandler{}

func NewVMRestoreRequestedHandler(useCase *UseCase, producer domain.Producer, nodeName string, logger *slog.Logger) *VMRestoreRequestedHandler {
	return &VMRestoreRequestedHandler{useCase: useCase, producer: producer, nodeName: nodeName, logger: logger}
}

func (h *VMRestoreRequestedHandler) Handle(ctx context.Context, data []byte) error {
	var requested events.VMRestoreRequested
	if err := json.Unmarshal(data, &requested); err != nil {
		h.logger.ErrorContext(ctx, "a vm could not be restored from an unreadable message", "error", err)

		return nil
	}

	if requested.NodeName != h.nodeName {
		return nil
	}

	response, err := h.useCase.Execute(ctx, &Request{
		VMUUID:       requested.VMUUID,
		SnapshotUUID: requested.SnapshotUUID,
		Spec:         requested.Spec.ToVM(),
	})
	if err != nil {
		return err
	}

	return vmcommand.Refused(ctx, h.producer, h.nodeName, requested.VMUUID, response.ValidationErrors)
}
