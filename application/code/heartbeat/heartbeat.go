package heartbeat

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type heartbeat struct {
	asyncReplyChan chan<- *domain.Reply
}

var _ domain.MessageHandler = &heartbeat{}

func NewHeartbeatHandler(asyncReplyChan chan<- *domain.Reply) *heartbeat {
	return &heartbeat{
		asyncReplyChan: asyncReplyChan,
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

	if task.IsTerminalState(taskState) {
		h.asyncReplyChan <- &domain.Reply{
			RequestID: requestID,
			Payload:   payload,
		}
	}

	return nil
}
