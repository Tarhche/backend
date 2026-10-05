package runStackAction

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
)

// StackRequestedHandler runs the stack actions the control plane asks of the
// Docker VMs on this node.
type StackRequestedHandler struct {
	useCase  *UseCase
	nodeName string
	logger   *slog.Logger
}

var _ domain.MessageHandler = &StackRequestedHandler{}

func NewStackRequestedHandler(useCase *UseCase, nodeName string, logger *slog.Logger) *StackRequestedHandler {
	return &StackRequestedHandler{useCase: useCase, nodeName: nodeName, logger: logger}
}

func (h *StackRequestedHandler) Handle(ctx context.Context, data []byte) error {
	var requested events.StackRequested
	if err := json.Unmarshal(data, &requested); err != nil {
		h.logger.ErrorContext(ctx, "a stack action could not be run from an unreadable message", "error", err)

		return nil
	}

	if requested.NodeName != h.nodeName {
		return nil
	}

	request := &Request{
		StackUUID:     requested.StackUUID,
		VMUUID:        requested.VMUUID,
		Action:        requested.Action,
		Project:       requested.Project,
		Compose:       requested.Compose,
		RemoveVolumes: requested.RemoveVolumes,
	}

	response, err := h.useCase.Execute(ctx, request)
	if err != nil {
		return err
	}

	return h.useCase.refused(ctx, request, response.ValidationErrors)
}
