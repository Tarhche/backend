// Package heartbeat tells whoever ran a piece of code what became of it.
//
// A job the code runner started is named after the request that asked for it,
// so what the runner says about that job is the answer to that request: the
// output it wrote when it ran, or why it never got to run at all.
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

	// ingressDomain is what the runner answers a container's ports under, so
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
// kinds is a job, which is what every container here was.
func kindOf(h *events.Heartbeat) task.Kind {
	if kind := task.Kind(h.Kind); kind.IsValid() {
		return kind
	}

	return task.DefaultKind
}

// deadline is when a snippet being watched will be stopped. The runner sets it
// as the container is made and reports it with every beat; a snippet that is
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
	// that asked for it. A service is a container from the dashboard, whose
	// name is a name: answering it would be answering a request nobody made.
	if kindOf(&heartbeat) != task.KindJob {
		return nil
	}

	taskState := task.State(heartbeat.State)
	requestID := heartbeat.Name

	h.logger.Info("heartbeat received", "heartbeat", heartbeat)

	response := &Response{
		Name:          heartbeat.Name,
		Logs:          heartbeat.Logs,
		State:         taskState.String(),
		ContainerUUID: heartbeat.UUID,
		Endpoints:     h.endpoints(&heartbeat, taskState),
		Deadline:      deadline(&heartbeat, taskState),
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

// endpoints are the addresses a running snippet answers on. A container that
// has ended answers on none, so they go with it rather than being left on the
// page pointing at nothing.
func (h *heartbeat) endpoints(beat *events.Heartbeat, state task.State) []Endpoint {
	if state != task.Running || len(beat.Slug) == 0 {
		return nil
	}

	endpoints := make([]Endpoint, 0, len(beat.Endpoints))
	for i, e := range beat.Endpoints {
		// the first port answers on the container's bare name, and the rest
		// carry their port in it: one name, one address.
		host := fmt.Sprintf("%s-%d.%s", beat.Slug, e.ContainerPort, h.ingressDomain)
		if i == 0 {
			host = fmt.Sprintf("%s.%s", beat.Slug, h.ingressDomain)
		}

		endpoints = append(endpoints, Endpoint{
			ContainerPort: uint(e.ContainerPort),
			URL:           "http://" + host,
		})
	}

	return endpoints
}
