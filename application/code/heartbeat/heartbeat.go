package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type heartbeat struct {
	replyer domain.Replyer
	logger  *slog.Logger
}

var _ domain.MessageHandler = &heartbeat{}

func NewHeartbeatHandler(replyer domain.Replyer, logger *slog.Logger) *heartbeat {
	return &heartbeat{
		replyer: replyer,
		logger:  logger,
	}
}

func (h *heartbeat) Handle(ctx context.Context, data []byte) error {
	var heartbeat events.Heartbeat
	if err := json.Unmarshal(data, &heartbeat); err != nil {
		return err
	}

	response := &Response{
		Name: heartbeat.Name,
		Logs: heartbeat.Logs,
	}

	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}

	taskState := task.State(heartbeat.State)
	requestID := heartbeat.Name

	h.logger.Info("heartbeat received", "heartbeat", heartbeat)

	if task.IsTerminalState(taskState) {
		if err := h.replyer.Reply(ctx, &domain.Reply{
			RequestID: requestID,
			Payload:   payload,
		}); err != nil {
			return err
		}
	}

	return nil
}
