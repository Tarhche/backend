// Package beatHeart says, every beat, what this node offers and what every
// kind it runs holds on it.
package beatHeart

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
)

// UseCase is this node's heartbeat.
//
// What the node offers is what the control plane places work by. What it
// holds is every kind's state action, asked of all of them at once, each for
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
	}
}

func (h *UseCase) Execute(ctx context.Context) error {
	nodeStats, err := h.nodeManager.Stats(ctx, h.nodeName)
	if err != nil {
		return err
	}

	at := time.Now()

	heartbeat := events.Heartbeat{
		Name:         h.nodeName,
		Role:         node.OrchestratorRole,
		Stats:        nodeStats,
		At:           at,
		Observations: h.observe(ctx),
	}

	payload, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}

	return h.producer.Produce(ctx, events.HeartbeatName, payload)
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
