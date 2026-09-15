package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type heartbeat struct {
	replyer domain.Replyer

	// ingressDomain is what the runner answers a task's ports under, so
	// that an exposed port becomes an address a reader can open.
	ingressDomain string

	logger *slog.Logger
}

var _ domain.MessageHandler = &heartbeat{}

func NewHeartbeatHandler(replyer domain.Replyer, ingressDomain string, logger *slog.Logger) *heartbeat {
	return &heartbeat{
		replyer:       replyer,
		ingressDomain: ingressDomain,
		logger:        logger,
	}
}

// kindOf reads what a heartbeat is reporting on. One from before there were
// kinds is a job, which is what every task here was.
func kindOf(h *events.Heartbeat) task.Kind {
	if kind := task.Kind(h.Kind); kind.IsValid() {
		return kind
	}

	return task.DefaultKind
}

// deadline is when a snippet being watched will be stopped. The runner sets it
// as the task is made and reports it with every beat; a snippet that is
// not running any more has none left to report.
func deadline(h *events.Heartbeat, state task.State) *time.Time {
	if !h.Interactive || state != task.Running || h.Deadline.IsZero() {
		return nil
	}

	at := h.Deadline

	return &at
}

func (h *heartbeat) Handle(ctx context.Context, data []byte) error {
	var heartbeat events.Heartbeat
	if err := json.Unmarshal(data, &heartbeat); err != nil {
		return err
	}

	// a job is a piece of code somebody ran here, and its name is the request
	// that asked for it. A service is a task from the dashboard, whose
	// name is a name: answering it would be answering a request nobody made.
	if kindOf(&heartbeat) != task.KindJob {
		return nil
	}

	taskState := task.State(heartbeat.State)
	requestID := heartbeat.Name

	h.logger.Info("heartbeat received", "heartbeat", heartbeat)

	response := &Response{
		Name:      heartbeat.Name,
		Logs:      heartbeat.Logs,
		State:     taskState.String(),
		TaskUUID:  heartbeat.UUID,
		Endpoints: h.endpoints(&heartbeat, taskState),
		Deadline:  deadline(&heartbeat, taskState),
	}

	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}

	// a snippet nobody is watching is answered once, with what it printed:
	// that is the whole of what somebody who ran it is waiting for.
	if !heartbeat.Interactive {
		if !task.IsTerminalState(taskState) {
			return nil
		}

		return h.replyer.Reply(ctx, &domain.Reply{
			RequestID: requestID,
			Payload:   payload,
		})
	}

	// one that is watched is told what it is doing as it does it, and where it
	// can be reached while it can be, until it ends.
	kind := domain.ReplyChunk
	if task.IsTerminalState(taskState) {
		kind = domain.ReplyEOF
	}

	return h.replyer.Reply(ctx, &domain.Reply{
		RequestID: requestID,
		Kind:      kind,
		Payload:   payload,
	})
}

func (h *heartbeat) endpoints(beat *events.Heartbeat, state task.State) []Endpoint {
	if state != task.Running || len(beat.Slug) == 0 {
		return nil
	}

	endpoints := make([]Endpoint, 0, len(beat.Endpoints))
	for _, e := range beat.Endpoints {
		host := fmt.Sprintf("%s-%d.%s", beat.Slug, e.TaskPort, h.ingressDomain)

		endpoints = append(endpoints, Endpoint{
			TaskPort: uint(e.TaskPort),
			URL:      "http://" + host,
		})
	}

	return endpoints
}
