// Package beatHeart says, every beat, what this node offers, and what every
// kind it runs holds on it, each kind in a heartbeat of its own.
package beatHeart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
)

// UseCase is this node's heartbeat.
//
// What the node offers, its stats and its capacity for VMs, is what the
// control plane places work by, and is said in the node's own heartbeat
// (events.Heartbeat), which says it is alive: first, before any kind is asked
// anything, so that no kind holds it up. What it holds is every kind's state
// action, asked of all of them at once, each for at most the state timeout,
// and each kind that answers says it in a heartbeat of its own
// (kind.Heartbeat), on the kind's own subject: a service hears only the kinds
// it needs, and a kind slow to answer holds up neither the node's beat nor
// the other kinds.
//
// A kind that fails, that takes longer than that, or that is still answering
// an earlier beat, sends nothing this beat, and the log says why: a kind that
// says nothing could not look, and nothing is concluded from its silence,
// while one that reports nothing holds nothing. So a kind that could not look
// is never reported as one whose resources are all gone. A kind whose state is
// known in the control plane, as a snapshot's is, holds nothing on a node to
// report, and is not asked. A heartbeat that cannot be sent keeps none of the
// others from being.
//
// A beat is stamped with when the kinds were asked, every heartbeat of it,
// the node's and each kind's, alike, so whatever it reports was observed no
// earlier than its stamp: a command's result stamped after it is newer than
// anything in it.
//
// A kind some of whose changes somebody waits on as they happen (kind.Prompt),
// a code-runner snippet ending, is asked between beats too (Hurry), and what
// it holds is reported at once, in a heartbeat of its own, when that changed
// since it was last reported.
type UseCase struct {
	producer    domain.Producer
	nodeManager node.Manager
	nodeName    string

	kinds        *kind.Registry[kind.NodeBinding]
	stateTimeout time.Duration
	logger       *slog.Logger

	// asking are the kinds whose state an earlier beat asked for and has
	// not had back yet. Such a kind is not asked again until it answers,
	// so one that does not let go is asked once rather than once a beat.
	lock   sync.Mutex
	asking map[string]bool

	// reported is what each kind last said it holds, as it was reported:
	// what a prompt kind is held to.
	reported map[string][]byte
}

// NewUseCase is the heartbeat of nodeName, reporting what kinds hold, each
// given stateTimeout to say it.
func NewUseCase(
	producer domain.Producer,
	nodeManager node.Manager,
	kinds *kind.Registry[kind.NodeBinding],
	stateTimeout time.Duration,
	nodeName string,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		producer:     producer,
		nodeManager:  nodeManager,
		nodeName:     nodeName,
		kinds:        kinds,
		stateTimeout: stateTimeout,
		logger:       logger,
		asking:       make(map[string]bool),
		reported:     make(map[string][]byte),
	}
}

func (h *UseCase) Execute(ctx context.Context) error {
	nodeStats, err := h.nodeManager.Stats(ctx, h.nodeName)
	if err != nil {
		return err
	}

	capacity, err := h.nodeManager.Capacity(ctx)
	if err != nil {
		return err
	}

	at := time.Now()

	beaten := h.send(ctx, events.HeartbeatName, events.Heartbeat{
		Name:     h.nodeName,
		Role:     node.OrchestratorRole,
		Stats:    nodeStats,
		Capacity: capacity,
		At:       at,
	})

	return errors.Join(beaten, h.report(ctx, at, h.observe(kind.WithBeat(ctx, at))))
}

// Prompt is how often Hurry is to be asked: as often as the promptest kind
// this node runs asks, and never when none is prompt.
func (h *UseCase) Prompt() time.Duration {
	var every time.Duration

	for _, binding := range h.kinds.All() {
		if prompt := binding.Prompt(); prompt > 0 && (every == 0 || prompt < every) {
			every = prompt
		}
	}

	return every
}

// Hurry asks every prompt kind this node runs what it holds, and reports at
// once, each in a heartbeat of its own, those that hold something other than
// they last reported: a snippet that ended, or came up and serves its ports,
// is told of as it happens rather than at the next beat. A kind still
// answering an earlier ask is left for the next.
func (h *UseCase) Hurry(ctx context.Context) error {
	at := time.Now()

	changed := make(map[string]kind.Report[json.RawMessage])

	for _, binding := range h.kinds.All() {
		name := binding.Descriptor().Name

		if binding.Prompt() <= 0 || binding.Descriptor().StateBy != kind.OnNode || !h.ask(name) {
			continue
		}

		asking, cancel := context.WithTimeout(ctx, h.stateTimeout)
		report, err := binding.State(asking)
		cancel()
		h.answered(name)

		if err != nil {
			h.logger.DebugContext(ctx, "a prompt kind could not say what it holds between beats", "kind", name, "error", err)

			continue
		}

		if h.changed(name, report) {
			changed[name] = report
		}
	}

	return h.report(ctx, at, changed)
}

// report sends what each kind holds in a heartbeat of its own, stamped at,
// and remembers what each was reported holding. One that cannot be sent
// keeps none of the others from being.
func (h *UseCase) report(ctx context.Context, at time.Time, reports map[string]kind.Report[json.RawMessage]) error {
	var failed error

	for _, name := range slices.Sorted(maps.Keys(reports)) {
		report := reports[name]

		if err := h.send(ctx, kind.HeartbeatName(name), kind.Heartbeat{Node: h.nodeName, Kind: name, At: at, Report: report}); err != nil {
			failed = errors.Join(failed, fmt.Errorf("kind %q: %w", name, err))

			continue
		}

		if said, err := json.Marshal(report); err == nil {
			h.lock.Lock()
			h.reported[name] = said
			h.lock.Unlock()
		}
	}

	return failed
}

// send says message on subject.
func (h *UseCase) send(ctx context.Context, subject string, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return h.producer.Produce(ctx, subject, payload)
}

// changed reports whether a kind holds something other than it was last
// reported holding.
func (h *UseCase) changed(name string, report kind.Report[json.RawMessage]) bool {
	said, err := json.Marshal(report)
	if err != nil {
		return true
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	return !bytes.Equal(h.reported[name], said)
}

// answer is what one kind said it holds, or why it could not.
type answer struct {
	kind   string
	report kind.Report[json.RawMessage]
	err    error
}

// observe is what every kind whose state is known on this node holds here,
// by kind, leaving out those that could not say in time. Nothing at all is no
// kind to ask, which is a node that runs none.
func (h *UseCase) observe(ctx context.Context) map[string]kind.Report[json.RawMessage] {
	var stated []kind.NodeBinding

	for _, binding := range h.kinds.All() {
		if binding.Descriptor().StateBy == kind.OnNode {
			stated = append(stated, binding)
		}
	}

	if len(stated) == 0 {
		return nil
	}

	asking, cancel := context.WithTimeout(ctx, h.stateTimeout)
	defer cancel()

	// buffered for every kind, so one answering after the beat went without
	// it is not left waiting to be heard.
	answers := make(chan answer, len(stated))
	waiting := make(map[string]bool, len(stated))

	for _, binding := range stated {
		name := binding.Descriptor().Name

		if !h.ask(name) {
			h.logger.WarnContext(ctx, "a kind is still saying what it holds from an earlier beat, and is left out of this one", "kind", name)

			continue
		}

		waiting[name] = true

		go func() {
			report, err := binding.State(asking)

			// free to be asked again before its answer is heard, so the beat
			// that hears it is never ahead of the kind being free.
			h.answered(name)

			answers <- answer{kind: name, report: report, err: err}
		}()
	}

	observations := make(map[string]kind.Report[json.RawMessage], len(waiting))

	for len(waiting) > 0 {
		select {
		case said := <-answers:
			delete(waiting, said.kind)

			if said.err != nil {
				h.logger.WarnContext(ctx, "a kind could not say what it holds, and is left out of this beat", "kind", said.kind, "error", said.err)

				continue
			}

			observations[said.kind] = said.report
		case <-asking.Done():
			for name := range waiting {
				h.logger.WarnContext(ctx, "a kind did not say what it holds in time, and is left out of this beat", "kind", name, "timeout", h.stateTimeout)
			}

			return observations
		}
	}

	return observations
}

// ask reports whether a kind may be asked for its state now, and marks it
// as being asked if so.
func (h *UseCase) ask(name string) bool {
	h.lock.Lock()
	defer h.lock.Unlock()

	if h.asking[name] {
		return false
	}

	h.asking[name] = true

	return true
}

// answered marks a kind as having answered, so the next beat asks it again.
func (h *UseCase) answered(name string) {
	h.lock.Lock()
	defer h.lock.Unlock()

	delete(h.asking, name)
}
