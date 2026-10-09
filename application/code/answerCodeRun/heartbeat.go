package answerCodeRun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/workload/node/events"
)

type heartbeat struct {
	replyer domain.Replyer

	// ingressDomain is what the workload answers a task's ports under, so
	// that an exposed port becomes an address a reader can open.
	ingressDomain string

	logger *slog.Logger
}

var _ domain.MessageHandler = &heartbeat{}

// NewHeartbeatHandler answers readers from the nodes' heartbeats.
func NewHeartbeatHandler(replyer domain.Replyer, ingressDomain string, logger *slog.Logger) *heartbeat {
	return &heartbeat{
		replyer:       replyer,
		ingressDomain: ingressDomain,
		logger:        logger,
	}
}

func (h *heartbeat) Handle(ctx context.Context, data []byte) error {
	var beat nodeEvents.Heartbeat
	if err := json.Unmarshal(data, &beat); err != nil {
		// read again, it is as unreadable; the next beat says it all again.
		h.logger.WarnContext(ctx, "a heartbeat that cannot be read", "error", err)

		return nil
	}

	report, reported := beat.Observations[taskKind.Name]
	if !reported {
		return nil
	}

	var failed error

	for _, observed := range report.Instances {
		var status taskKind.Status
		if err := json.Unmarshal(observed.Status, &status); err != nil {
			h.logger.WarnContext(ctx, "a task's status that cannot be read", "error", err, "task", observed.UUID)

			continue
		}

		failed = errors.Join(failed, h.answer(ctx, observed.UUID, status))
	}

	return failed
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

	response := &Response{
		Name:      run.Name,
		State:     string(status.State),
		TaskUUID:  uuid,
		Endpoints: h.endpoints(run, status.State),
		Deadline:  deadline(run, status.State),
	}

	if len(run.Output) > 0 {
		response.Logs = []byte(run.Output)
	}

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

// deadline is when a snippet being watched will be stopped. Its node sets it
// as the snippet's run comes up and reports it with every beat; a snippet that
// is not running any more has none left to report.
func deadline(run *taskKind.Run, state kind.State) *time.Time {
	if !run.Interactive || state != taskKind.Running || run.Deadline.IsZero() {
		return nil
	}

	at := run.Deadline

	return &at
}

// endpoints are where a running snippet answers: each of its ports that came
// up, under its slug and the workload's domain.
func (h *heartbeat) endpoints(run *taskKind.Run, state kind.State) []Endpoint {
	if state != taskKind.Running || len(run.Slug) == 0 {
		return nil
	}

	endpoints := make([]Endpoint, 0, len(run.Endpoints))
	for _, e := range run.Endpoints {
		host := fmt.Sprintf("%s-%d.%s", run.Slug, e.Port, h.ingressDomain)

		endpoints = append(endpoints, Endpoint{
			TaskPort: uint(e.Port),
			URL:      "http://" + host,
		})
	}

	return endpoints
}
