package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
)

// result answers whoever ran a snippet that could not be run at all: its
// task's node could not make the run, and said why as what came of the
// command to make it. A snippet that ran and failed speaks for itself
// through its own output, at a heartbeat.
//
// What a node says of a create that failed is the run it was to be, which
// is whose request it answers. A snippet's task is never asked for again
// after it fails, so its first failure is its last, and is answered.
type result struct {
	replyer domain.Replyer
	logger  *slog.Logger
}

var _ domain.MessageHandler = &result{}

// NewResultHandler answers readers from what came of the commands to run
// their snippets.
func NewResultHandler(replyer domain.Replyer, logger *slog.Logger) *result {
	return &result{replyer: replyer, logger: logger}
}

func (h *result) Handle(ctx context.Context, data []byte) error {
	var answered kind.Result
	if err := json.Unmarshal(data, &answered); err != nil {
		// read again, it is as unreadable.
		h.logger.WarnContext(ctx, "a command's result that cannot be read", "error", err)

		return nil
	}

	if answered.Kind != taskKind.Name || answered.Action != taskKind.ActionCreate || answered.OK || len(answered.Reason) == 0 || len(answered.Status) == 0 {
		return nil
	}

	var status taskKind.Status
	if err := json.Unmarshal(answered.Status, &status); err != nil {
		h.logger.WarnContext(ctx, "a task's status that cannot be read", "error", err, "task", answered.UUID)

		return nil
	}

	run := status.Run
	if !run.Job() || len(run.Name) == 0 {
		return nil
	}

	h.logger.WarnContext(ctx, "a piece of code never ran", "reason", answered.Reason, "request", run.Name)

	payload, err := json.Marshal(&Response{
		Name:  run.Name,
		Error: answered.Reason,
	})
	if err != nil {
		return err
	}

	return h.replyer.Reply(ctx, &domain.Reply{
		RequestID: run.Name,
		Payload:   payload,
	})
}
