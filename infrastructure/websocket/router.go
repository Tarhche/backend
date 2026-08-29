package websocket

import (
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// router gives every connection the machinery it needs to carry one client's
// conversation: a registry of its own, a validator and dispatcher that hold that
// registry, and the session that drives them.
//
// The registry is per connection, so the request ids a client picks are its own,
// two clients may use the same ones, and a session can only ever resolve a reply
// to a request it registered itself.
type router struct {
	producer     domain.Producer
	subjects     *subjects
	hub          *hub
	bus          *replyBus
	registries   func() RequestRegistry
	replyBackoff Backoff
	queueBackoff Backoff
	translator   translator.Translator
	logger       *slog.Logger
}

// newSession wires a session around one client's connection.
func (r *router) newSession(c conn) *session {
	requests := r.registries()

	return &session{
		conn: c,
		dispatcher: &dispatcher{
			validator: newRequestValidator(requests, r.subjects, r.translator),
			registry:  requests,
			producer:  r.producer,
			logger:    r.logger,
		},
		registry:     requests,
		hub:          r.hub,
		bus:          r.bus,
		replyBackoff: r.replyBackoff,
		queueBackoff: r.queueBackoff,
		translator:   r.translator,
		logger:       r.logger,
	}
}
