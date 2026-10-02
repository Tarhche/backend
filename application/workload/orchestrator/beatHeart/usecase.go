// Package beatHeart tells the control plane this node is here, what its runs
// use, and which classes it offers: whether each can run tasks right now,
// what it can do and how much room it has.
package beatHeart

import (
	"context"
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

type UseCase struct {
	producer    domain.Producer
	nodeManager node.Manager
	drivers     driver.Set
	nodeName    string
}

func NewUseCase(
	producer domain.Producer,
	nodeManager node.Manager,
	drivers driver.Set,
	nodeName string,
) *UseCase {
	return &UseCase{
		producer:    producer,
		nodeManager: nodeManager,
		drivers:     drivers,
		nodeName:    nodeName,
	}
}

func (h *UseCase) Execute(ctx context.Context) error {
	nodeStats, err := h.nodeManager.Stats(ctx, h.nodeName)
	if err != nil {
		return err
	}

	heartbeat := events.Heartbeat{
		Name:     h.nodeName,
		Role:     node.OrchestratorRole,
		Stats:    nodeStats,
		Runtimes: h.offers(ctx),
		At:       time.Now(),
	}

	payload, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}

	return h.producer.Produce(ctx, events.HeartbeatName, payload)
}

// offers is every class this node offers, in the order they were configured.
//
// A class that cannot run tasks right now is offered all the same, unhealthy
// and saying why: it is what tells the control plane that what the node runs
// under it has not gone, but cannot be heard from, and that nothing new should
// be placed there until it can.
func (h *UseCase) offers(ctx context.Context) []runtime.Offer {
	drivers := h.drivers.All()

	offers := make([]runtime.Offer, 0, len(drivers))
	for _, d := range drivers {
		offers = append(offers, d.Offer(ctx))
	}

	return offers
}
