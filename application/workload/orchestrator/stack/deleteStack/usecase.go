// Package deleteStack drops the private network a stack's services shared, once
// those services are gone.
package deleteStack

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
)

type StackDeletedHandler struct {
	drivers driver.Set

	// everyClass is the networks of every class at once, for a stack whose
	// class the message does not say.
	everyClass network.Manager

	nodeName string
	logger   *slog.Logger
}

var _ domain.MessageHandler = &StackDeletedHandler{}

func NewStackDeletedHandler(drivers driver.Set, everyClass network.Manager, nodeName string, logger *slog.Logger) *StackDeletedHandler {
	return &StackDeletedHandler{
		drivers:    drivers,
		everyClass: everyClass,
		nodeName:   nodeName,
		logger:     logger,
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

	networks, found := h.networksOf(ctx, &deleted)
	if !found {
		return nil
	}

	// the removal waits for the stack's tasks to detach, so reaching here
	// with an error means the network is genuinely stuck. Reported rather than
	// redelivered: nothing about trying the same thing again would free it, and
	// a stuck network is a leak to look at rather than a message to replay.
	if err := networks.RemoveStackNetwork(ctx, deleted.Slug); err != nil {
		h.logger.ErrorContext(ctx, "a stack's network outlived it", "error", err, "stack", deleted.Slug, "runtime", deleted.Runtime)
	}

	return nil
}

// networksOf is the networks the stack's network belongs to: its class's,
// since a stack's network is made by the driver of the stack's class.
//
// A message that names no class comes from a control plane older than classes,
// and every class is asked: one that has no such network has nothing to drop.
// A class this node does not offer has nothing here to drop either, though
// the stack ran here: the class was taken off the node with the stack still on
// it, which is worth saying.
func (h *StackDeletedHandler) networksOf(ctx context.Context, deleted *events.StackDeleted) (network.Manager, bool) {
	if len(deleted.Runtime) == 0 {
		return h.everyClass, true
	}

	d, err := h.drivers.For(deleted.Runtime)
	if err != nil {
		h.logger.WarnContext(ctx, "a stack's network belongs to a class this node does not offer", "error", err, "stack", deleted.Slug, "runtime", deleted.Runtime)

		return nil, false
	}

	return d.Networks(), true
}
