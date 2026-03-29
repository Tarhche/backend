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
	producer domain.Producer
	nodeName string
}

func NewUseCase(
	producer domain.Producer,
	nodeName string,
) *UseCase {
	return &UseCase{
		producer: producer,
		nodeName: nodeName,
	}
}

func (h *UseCase) Execute() error {
	heartbeat := events.Heartbeat{
		Name: h.nodeName,
		Role: node.Worker,
		At:   time.Now(),
	}

	payload, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}

	return h.producer.Produce(context.Background(), events.HeartbeatName, payload)
}
