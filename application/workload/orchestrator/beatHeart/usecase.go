// Package beatHeart says, every beat, what this node offers and what every
// kind it runs holds on it.
package beatHeart

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase is this node's heartbeat.
//
// What the node offers, its stats and its capacity for VMs, is what the
// control plane places work by. What it holds is every kind's state action, asked of all of them at once, each for
// at most the state timeout, so a kind slow to answer holds up neither the
// beat nor the other kinds.
//
// A kind that fails, that takes longer than that, or that is still answering
// an earlier beat, is left out of this one, and the log says why: a kind that
// is not in a heartbeat said nothing this beat, and nothing is concluded from
// its silence, while one that is there with nothing in it holds nothing. So a
// kind that could not look is never reported as one whose resources are all
// gone. A kind whose state is known in the control plane, as a snapshot's is,
// holds nothing on a node to report, and is not asked.
//
// A beat is stamped with when the kinds were asked, so whatever it reports
// was observed no earlier than its stamp: a command's result stamped after it
// is newer than anything in it.
//
// A kind some of whose changes somebody waits on as they happen (kind.Prompt),
// a code-runner snippet ending, is asked between beats too (Hurry), and what
// it holds is reported at once, in a beat of its own, when that changed since
// it was last reported. Such a beat carries that kind alone, beside what the
// node offered at the last beat: the kinds it leaves out said nothing, as at
// any beat, and nothing is concluded about them.
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

	// reported is what each kind last said it holds, as it was reported, and
	// offered what the node offered at its last beat, which a beat between
	// beats says again: what a prompt kind is held to, and what its beats are
	// sent with.
	reported map[string][]byte
	offered  *offer
}

// offer is what a node offers, as a beat says it.
type offer struct {
	stats    node.Stats
	capacity vm.Info
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

	heartbeat := events.Heartbeat{
		Name:         h.nodeName,
		Role:         node.OrchestratorRole,
		Stats:        nodeStats,
		Capacity:     capacity,
		At:           at,
		Observations: h.observe(kind.WithBeat(ctx, at)),
	}

	if err := h.beat(ctx, heartbeat); err != nil {
		return err
	}

	h.lock.Lock()
	h.offered = &offer{stats: nodeStats, capacity: capacity}
	h.lock.Unlock()

	return nil
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
// once, in a beat of its own, those that hold something other than they last
// reported: a snippet that ended, or came up and serves its ports, is told of
// as it happens rather than at the next beat. A kind still answering an
// earlier ask is left for the next, and nothing is reported before the node's
// first beat has said what it offers.
func (h *UseCase) Hurry(ctx context.Context) error {
	h.lock.Lock()
	offered := h.offered
	h.lock.Unlock()

	if offered == nil {
		return nil
	}

	at := time.Now()

	observations := make(map[string]kind.Report[json.RawMessage])

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
			observations[name] = report
		}
	}

	if len(observations) == 0 {
		return nil
	}

	return h.beat(ctx, events.Heartbeat{
		Name:         h.nodeName,
		Role:         node.OrchestratorRole,
		Stats:        offered.stats,
		Capacity:     offered.capacity,
		At:           at,
		Observations: observations,
	})
}

// beat sends a heartbeat, and remembers what each kind in it was reported
// holding.
func (h *UseCase) beat(ctx context.Context, heartbeat events.Heartbeat) error {
	payload, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}

	if err := h.producer.Produce(ctx, events.HeartbeatName, payload); err != nil {
		return err
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	for name, report := range heartbeat.Observations {
		if said, err := json.Marshal(report); err == nil {
			h.reported[name] = said
		}
	}

	return nil
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
