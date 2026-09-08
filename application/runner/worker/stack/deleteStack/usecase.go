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

	if err := h.networkManager.RemoveStackNetwork(ctx, deleted.Slug); err != nil {
		// the containers are already gone; a network that will not go yet is
		// worth reporting but not worth redelivering forever.
		h.logger.WarnContext(ctx, "could not remove a stack's network", "error", err, "stack", deleted.Slug)
	}

	return nil
}
