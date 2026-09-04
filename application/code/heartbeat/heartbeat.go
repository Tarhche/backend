// Package heartbeat tells whoever ran a piece of code what became of it.
//
// A job the code runner started is named after the request that asked for it,
// so what the runner says about that job is the answer to that request: the
// output it wrote when it ran, or why it never got to run at all.
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

// kindOf reads what a heartbeat is reporting on. One from before there were
// kinds is a job, which is what every container here was.
func kindOf(h *events.Heartbeat) task.Kind {
	if kind := task.Kind(h.Kind); kind.IsValid() {
		return kind
	}

	return task.DefaultKind
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

	// a job is a piece of code somebody ran here, and its name is the request
	// that asked for it. A service is a container from the dashboard, whose
	// name is a name: answering it would be answering a request nobody made.
	if kindOf(&heartbeat) != task.KindJob {
		return nil
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
