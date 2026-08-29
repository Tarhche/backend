package websocket

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// subjectsPrefix namespaces the websocket's subjects on the broker.
const subjectsPrefix = "websocket_"

// Websocket wires up the path a request takes and the path its reply takes back:
//
//	client -> session -> dispatcher -> broker (produced once, for one replica)
//	broker (published to every replica) -> replyBus -> hub -> session -> client
//
// It performs none of those steps itself; it assembles them and gives every
// connection its own session.
type Websocket struct {
	// a ProduceConsumer is two roles: the websocket consumes, the dispatcher
	// produces, so each holds the half it needs.
	consumer   domain.Consumer
	dispatcher *dispatcher
	registry   domain.RequestRegistry
	subjects   *subjects
	translator translator.Translator
	hub        *hub
	bus        *replyBus
	upgrader   websocket.Upgrader
	config     configuration
	logger     *slog.Logger
}

// Ensure Websocket implements the domain.Consumer interface
var _ domain.Consumer = &Websocket{}

// Ensure Websocket implements the domain.Replyer interface
var _ domain.Replyer = &Websocket{}

// make sure the websocket implements the http.Handler interface
var _ http.Handler = &Websocket{}

// make sure the websocket implements the io.Closer interface
var _ io.Closer = &Websocket{}

func NewWebsocket(
	requestRegistry domain.RequestRegistry,
	produceConsumer domain.ProduceConsumer,
	publishSubscriber domain.PublishSubscriber,
	translator translator.Translator,
	repliesSubject string,
	logger *slog.Logger,
	options ...Option,
) (*Websocket, error) {
	config, err := newConfiguration(options...)
	if err != nil {
		return nil, err
	}

	subjects := newSubjects()

	w := &Websocket{
		consumer: produceConsumer,
		dispatcher: &dispatcher{
			validator: newRequestValidator(requestRegistry, subjects, translator),
			registry:  requestRegistry,
			producer:  produceConsumer,
			logger:    logger,
		},
		registry:   requestRegistry,
		subjects:   subjects,
		translator: translator,
		hub:        newHub(config.outboundBuffer, logger),
		bus:        newReplyBus(publishSubscriber, subjectsPrefix+repliesSubject, logger),
		upgrader:   websocket.Upgrader{CheckOrigin: config.checkOrigin},
		config:     config,
		logger:     logger,
	}

	if err := w.bus.start(context.Background()); err != nil {
		return nil, err
	}

	go w.fanoutReplies()

	return w, nil
}

// Reply hands a reply to every replica, because the waiting client may be
// connected to any of them.
func (w *Websocket) Reply(ctx context.Context, reply *domain.Reply) error {
	if w.bus.isClosed() {
		return ErrClosed
	}

	if len(reply.RequestID) == 0 {
		return ErrRequestIDRequired
	}

	return w.bus.publish(ctx, reply)
}

// Consume subscribes to the given subject and handles incoming messages using
// the provided handler once for each message, even if there are multiple
// replicas of the application running. Consuming a subject is also what allows
// clients to send requests to it.
func (w *Websocket) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	if err := w.consumer.Consume(ctx, subjectsPrefix+subject, handler); err != nil {
		return err
	}

	w.subjects.add(subject)

	return nil
}

func (w *Websocket) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	conn, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		w.logger.Error("failed to upgrade websocket connection", "error", err)

		return
	}

	w.logger.Info("new client connected", "remoteAddress", conn.RemoteAddr().String())

	session := &session{
		conn:       newConnection(conn, w.config, w.logger),
		dispatcher: w.dispatcher,
		registry:   w.registry,
		hub:        w.hub,
		bus:        w.bus,
		translator: w.translator,
		logger:     w.logger,
	}

	session.run(r.Context())
}

// Close stops carrying replies. Open connections end with their own requests.
func (w *Websocket) Close() error {
	return w.bus.shutdown()
}

// fanoutReplies hands every reply this replica receives to all of its sessions.
func (w *Websocket) fanoutReplies() {
	for {
		select {
		case <-w.bus.closed():
			return

		case reply := <-w.bus.receive():
			w.logger.Info("publishing reply to all response channels", "requestID", reply.RequestID)

			w.hub.broadcast(reply)
		}
	}
}
