// Package beatHeart says, every beat, what this node offers, and every
// instance it holds of every kind it runs, each in a heartbeat of its own.
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
// and every instance of each kind that answers is said in a heartbeat of its
// own (kind.Heartbeat), on its kind's own subject: a service hears only the
// kinds it needs, and a kind slow to answer holds up neither the node's beat
// nor the other kinds.
//
// A kind that fails, that takes longer than that, or that is still answering
// an earlier beat, sends nothing this beat, and the log says why. No heartbeat
// says what the node does not hold: whoever keeps a resource's record takes
// it to be gone once the node goes on beating without a word of it for long
// enough, so a kind that cannot say what it holds for that long has all of it
// taken to be gone, until it says it again. A kind whose state is known in
// the control plane, as a snapshot's is, holds nothing on a node to report,
// and is not asked. A heartbeat that cannot be sent keeps none of the others
// from being.
//
// A beat is stamped with when the kinds were asked, every heartbeat of it,
// the node's and each instance's, alike, so whatever it reports was observed
// no earlier than its stamp: a command's result stamped after it is newer than
// anything in it.
//
// A kind some of whose changes somebody waits on as they happen (kind.Prompt),
// a code-runner snippet ending, is asked between beats too (Hurry), and each
// of its instances that changed since it was last reported is reported at
// once, in a heartbeat of its own.
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

	// reported is what each instance of each kind was last reported as, by
	// kind and by what the instance is known by (identity): what a prompt
	// kind's instances are held to.
	reported map[string]map[string][]byte
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
		reported:     make(map[string]map[string][]byte),
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

	return errors.Join(beaten, h.report(ctx, at, h.observe(kind.WithBeat(ctx, at)), true))
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
// once, each in a heartbeat of its own, every instance that is other than it
// was last reported, or was not reported at all: a snippet that ended, or came
// up and serves its ports, is told of as it happens rather than at the next
// beat. A kind still answering an earlier ask is left for the next.
func (h *UseCase) Hurry(ctx context.Context) error {
	at := time.Now()

	held := make(map[string]kind.Report[json.RawMessage])

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

		held[name] = report
	}

	return h.report(ctx, at, held, false)
}

// report sends the instances each kind holds, every one of them when all
// says so and otherwise those other than they were last reported, each in a
// heartbeat of its own stamped at, and remembers what each kind's instances
// were reported as: what one no longer holds is forgotten. One that cannot be
// sent keeps none of the others from being, and is sent at the next chance.
func (h *UseCase) report(ctx context.Context, at time.Time, reports map[string]kind.Report[json.RawMessage], all bool) error {
	var failed error

	for _, name := range slices.Sorted(maps.Keys(reports)) {
		h.lock.Lock()
		before := h.reported[name]
		h.lock.Unlock()

		instances := reports[name].Instances
		reported := make(map[string][]byte, len(instances))

		for _, instance := range instances {
			said, err := json.Marshal(instance)
			if err != nil {
				failed = errors.Join(failed, fmt.Errorf("the %s %q cannot be written: %w", name, instance.UUID, err))

				continue
			}

			known := identity(instance, said)

			if !all && bytes.Equal(before[known], said) {
				reported[known] = said

				continue
			}

			if err := h.send(ctx, kind.HeartbeatName(name), kind.Heartbeat{Node: h.nodeName, At: at, Observed: instance}); err != nil {
				failed = errors.Join(failed, fmt.Errorf("the %s %q: %w", name, instance.UUID, err))

				continue
			}

			reported[known] = said
		}

		h.lock.Lock()
		h.reported[name] = reported
		h.lock.Unlock()
	}

	return failed
}

// identity is what an instance is known by among its kind's: the resource it
// is, or, for one nobody keeps a record of, everything observed of it, which
// is then told of again whenever any of it changes.
func identity(instance kind.Observation, said []byte) string {
	if len(instance.UUID) > 0 {
		return instance.UUID
	}

	return string(said)
}

// send says message on subject.
func (h *UseCase) send(ctx context.Context, subject string, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return h.producer.Produce(ctx, subject, payload)
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
