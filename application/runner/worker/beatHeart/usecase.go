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
	publisher domain.Publisher
	nodeName  string
}

func NewUseCase(
	publisher domain.Publisher,
	nodeName string,
) *UseCase {
	return &UseCase{
		publisher: publisher,
		nodeName:  nodeName,
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

	return h.publisher.Publish(context.Background(), events.HeartbeatName, payload)
}
