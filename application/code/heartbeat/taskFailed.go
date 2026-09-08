package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// taskFailed answers somebody whose code never ran.
//
// A job that fails before it has a container has no output and no heartbeat to
// carry one: without this, the request that asked for it is never answered and
// the page waits for something that is not coming.
type taskFailed struct {
	replyer domain.Replyer
	logger  *slog.Logger
}

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

	// only a failure the runner can explain is worth answering with: a
	// container that ran and exited badly has already said everything it has
	// to say, and its heartbeat carries that.
	if len(failed.Reason) == 0 || len(failed.Name) == 0 {
		return nil
	}

	// a failure the runner is going to try again is not an answer: the code
	// may still run, and whoever asked for it is told once, about whatever
	// finally became of it.
	if !failed.LastAttempt() {
		return nil
	}

	h.logger.WarnContext(ctx, "a piece of code never ran", "reason", failed.Reason, "request", failed.Name)

	payload, err := json.Marshal(&Response{Name: failed.Name, Error: failed.Reason})
	if err != nil {
		return err
	}

	return h.replyer.Reply(ctx, &domain.Reply{
		RequestID: failed.Name,
		Payload:   payload,
	})
}
