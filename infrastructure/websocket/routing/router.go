package routing

import (
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/protocol"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/transport"
)

// Backoffs are the retry policies a session delivers replies on.
type Backoffs struct {
	Reply Backoff
	Queue Backoff
}

// Router gives every connection the machinery it needs to carry one client's
// conversation: a registry of its own, a validator and dispatcher that hold that
// registry, and the session that drives them.
//
// The registry is per connection, so the request ids a client picks are its own,
// two clients may use the same ones, and a session can only ever resolve a reply
// to a request it registered itself.
type Router struct {
	Producer   domain.Producer
	Subjects   *protocol.Subjects
	Hub        *transport.Hub
	Bus        *transport.ReplyBus
	Backoffs   Backoffs
	Registries func() RequestRegistry
	Translator translator.Translator
	Logger     *slog.Logger
}

// DefaultRegistries hands every connection a registry of its own, sized for
// what one client is expected to have in flight at once.
func DefaultRegistries() func() RequestRegistry {
	return func() RequestRegistry { return NewInMemoryRegistry(8) }
}

// NewSession wires a session around one client's connection.
func (r *Router) NewSession(conn Conn) *Session {
	requests := r.Registries()

	return &Session{
		conn: conn,
		dispatcher: &Dispatcher{
			validator: protocol.NewValidator(requests, r.Subjects, r.Translator),
			registry:  requests,
			producer:  r.Producer,
			logger:    r.Logger,
		},
		registry:     requests,
		hub:          r.Hub,
		bus:          r.Bus,
		replyBackoff: r.Backoffs.Reply,
		queueBackoff: r.Backoffs.Queue,
		translator:   r.Translator,
		logger:       r.Logger,
	}
}
