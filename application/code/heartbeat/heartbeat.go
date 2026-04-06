package heartbeat

import (
	"context"
	"encoding/json"
	"log"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type heartbeat struct {
	replyer domain.Replyer
}

var _ domain.MessageHandler = &heartbeat{}

func NewHeartbeatHandler(replyer domain.Replyer) *heartbeat {
	return &heartbeat{
		replyer: replyer,
	}
}

func (h *heartbeat) Handle(data []byte) error {
	var heartbeat events.Heartbeat
	if err := json.Unmarshal(data, &heartbeat); err != nil {
		return err
	}

	response := &Response{
		UUID: heartbeat.UUID,
		Name: heartbeat.Name,
		Logs: heartbeat.Logs,
	}

	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}

	taskState := task.State(heartbeat.State)
	requestID := heartbeat.Name

	log.Printf("heartbeat: %+v", heartbeat)

	if task.IsTerminalState(taskState) {
		if err := h.replyer.Reply(context.Background(), &domain.Reply{
			RequestID: requestID,
			Payload:   payload,
		}); err != nil {
			return err
		}
	}

	return nil
}
