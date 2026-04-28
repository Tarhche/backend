package beatHeart

import (
	"context"
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/node/events"
)

type UseCase struct {
	producer    domain.Producer
	nodeManager node.Manager
	nodeName    string
}

func NewUseCase(
	producer domain.Producer,
	nodeManager node.Manager,
	nodeName string,
) *UseCase {
	return &UseCase{
		producer:    producer,
		nodeManager: nodeManager,
		nodeName:    nodeName,
	}
}

func (h *UseCase) Execute() error {
	nodeStats, err := h.nodeManager.Stats(h.nodeName)
	if err != nil {
		return err
	}

	heartbeat := events.Heartbeat{
		Name:  h.nodeName,
		Role:  node.WorkerRole,
		Stats: nodeStats,
		At:    time.Now(),
	}

	payload, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}

	return h.producer.Produce(context.Background(), events.HeartbeatName, payload)
}
