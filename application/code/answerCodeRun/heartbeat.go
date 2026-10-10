package answerCodeRun

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
)

type heartbeat struct {
	replyer       domain.Replyer
	ingressDomain string
	logger        *slog.Logger
}

var _ domain.MessageHandler = &heartbeat{}

// NewHeartbeatHandler answers readers from the task kind's heartbeats, each
// of which says what one of the nodes' tasks is doing.
func NewHeartbeatHandler(replyer domain.Replyer, ingressDomain string, logger *slog.Logger) *heartbeat {
	return &heartbeat{
		replyer:       replyer,
		ingressDomain: ingressDomain,
		logger:        logger,
	}
}

func (h *heartbeat) Handle(ctx context.Context, data []byte) error {
	var beat kind.Heartbeat
	if err := json.Unmarshal(data, &beat); err != nil {
		h.logger.WarnContext(ctx, "a heartbeat that cannot be read", "error", err)

		return nil
	}

	if beat.Kind != taskKind.Name {
		return nil
	}

	var status taskKind.Status
	if err := json.Unmarshal(beat.Status, &status); err != nil {
		h.logger.WarnContext(ctx, "a task's status that cannot be read", "error", err, "task", beat.UUID)

		return nil
	}

	return h.answer(ctx, beat.UUID, status)
}

// answer tells whoever ran a snippet what its task is doing, when there is
// something to tell them.
func (h *heartbeat) answer(ctx context.Context, uuid string, status taskKind.Status) error {
	run := status.Run

	// a job is a piece of code somebody ran here, and its name is the request
	// that asked for it. A service is a task of somebody's, whose name is a
	// name: answering it would be answering a request nobody made.
	if !run.Job() || len(run.Name) == 0 || len(status.State) == 0 {
		return nil
	}

	ended := taskKind.Ended(status.State)

	// a snippet nobody is watching is answered once, with what it printed:
	// that is the whole of what somebody who ran it is waiting for.
	if !run.Interactive && !ended {
		return nil
	}

	// 2. continue reviewing till the end.
	// 3. do we need to have a task kind????
	response := NewResponse(uuid, status, h.ingressDomain)

	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}

	reply := &domain.Reply{RequestID: run.Name, Payload: payload}

	// one that is watched is told what it is doing as it does it, and where it
	// can be reached while it can be, until it ends.
	if run.Interactive {
		reply.Kind = domain.ReplyChunk
		if ended {
			reply.Kind = domain.ReplyEOF
		}
	}

	return h.replyer.Reply(ctx, reply)
}
