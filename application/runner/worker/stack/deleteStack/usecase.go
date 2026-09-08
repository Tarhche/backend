// Package deleteStack drops the private network a stack's services shared, once
// those services are gone.
package deleteStack

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/stack/events"
)

type StackDeletedHandler struct {
	networkManager network.Manager
	nodeName       string
	logger         *slog.Logger
}

var _ domain.MessageHandler = &StackDeletedHandler{}

func NewStackDeletedHandler(networkManager network.Manager, nodeName string, logger *slog.Logger) *StackDeletedHandler {
	return &StackDeletedHandler{
		networkManager: networkManager,
		nodeName:       nodeName,
		logger:         logger,
	}
}

func (h *StackDeletedHandler) Handle(ctx context.Context, data []byte) error {
	var deleted events.StackDeleted
	if err := json.Unmarshal(data, &deleted); err != nil {
		return err
	}

	// the network is local to the node the stack ran on, so only that node has
	// anything to drop.
	if deleted.NodeName != h.nodeName || len(deleted.Slug) == 0 {
		return nil
	}

	// the removal waits for the stack's containers to detach, so reaching here
	// with an error means the network is genuinely stuck. Reported rather than
	// redelivered: nothing about trying the same thing again would free it, and
	// a stuck network is a leak to look at rather than a message to replay.
	if err := h.networkManager.RemoveStackNetwork(ctx, deleted.Slug); err != nil {
		h.logger.ErrorContext(ctx, "a stack's network outlived it", "error", err, "stack", deleted.Slug)
	}

	return nil
}
