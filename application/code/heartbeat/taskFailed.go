package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type taskFailed struct {
	replyer domain.Replyer
	logger  *slog.Logger
}

// Ensure taskFailed implements MessageHandler interface.
var _ domain.MessageHandler = &taskFailed{}

func NewTaskFailedHandler(replyer domain.Replyer, logger *slog.Logger) *taskFailed {
	return &taskFailed{
		replyer: replyer,
		logger:  logger,
	}
}

func (h *taskFailed) Handle(ctx context.Context, data []byte) error {
	var failed events.TaskFailed
	if err := json.Unmarshal(data, &failed); err != nil {
		return err
	}

	if len(failed.Reason) == 0 || len(failed.Name) == 0 {
		return nil
	}

	if !failed.LastAttempt() {
		return nil
	}

	h.logger.WarnContext(ctx, "a piece of code never ran", "reason", failed.Reason, "request", failed.Name)

	response := &Response{
		Name:  failed.Name,
		Error: failed.Reason,
	}

	payload, err := json.Marshal(&response)
	if err != nil {
		return err
	}

	return h.replyer.Reply(ctx, &domain.Reply{
		RequestID: failed.Name,
		Payload:   payload,
	})
}
