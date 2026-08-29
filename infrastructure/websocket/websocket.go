package websocket

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/protocol"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/routing"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/transport"
)

// Websocket wires up the path a request takes and the path its reply takes back:
//
//	client -> session -> dispatcher -> broker (produced once, for one replica)
//	broker (published to every replica) -> replyBus -> hub -> session -> client
//
// It performs none of those steps itself. transport carries the bytes, protocol
// decides what a client may say, routing gets each reply back to the connection
// that asked for it, and this assembles the three.
type Websocket struct {
	consumer domain.Consumer
	subjects *protocol.Subjects
	hub      *transport.Hub
	bus      *transport.ReplyBus
	router   *routing.Router
	upgrader websocket.Upgrader
	config   configuration
	logger   *slog.Logger
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

	var (
		subjects = protocol.NewSubjects()
		hub      = transport.NewHub(config.outboundBuffer, logger)
		bus      = transport.NewReplyBus(publishSubscriber, protocol.BrokerSubject(repliesSubject), logger)
	)

	w := &Websocket{
		consumer: produceConsumer,
		subjects: subjects,
		hub:      hub,
		bus:      bus,
		router: &routing.Router{
			Producer:   produceConsumer,
			Subjects:   subjects,
			Hub:        hub,
			Bus:        bus,
			Backoffs:   config.backoffs(),
			Registries: config.registries,
			Translator: translator,
			Logger:     logger,
		},
		upgrader: websocket.Upgrader{CheckOrigin: config.checkOrigin},
		config:   config,
		logger:   logger,
	}

	if err := w.bus.Start(context.Background()); err != nil {
		return nil, err
	}

	go w.fanoutReplies()

	return w, nil
}

// Reply hands a reply to every replica, because the waiting client may be
// connected to any of them.
func (w *Websocket) Reply(ctx context.Context, reply *domain.Reply) error {
	if w.bus.IsClosed() {
		return ErrClosed
	}

	if len(reply.RequestID) == 0 {
		return ErrRequestIDRequired
	}

	return w.bus.Publish(ctx, reply)
}

// Consume subscribes to the given subject and handles incoming messages using
// the provided handler once for each message, even if there are multiple
// replicas of the application running. Consuming a subject is also what allows
// clients to send requests to it.
func (w *Websocket) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	if err := w.consumer.Consume(ctx, protocol.BrokerSubject(subject), handler); err != nil {
		return err
	}

	w.subjects.Add(subject)

	return nil
}

func (w *Websocket) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// once the bus is closed nothing can carry a reply back, so accepting the
	// client would produce work whose answer it could never receive.
	if w.bus.IsClosed() {
		w.logger.Warn("refusing a websocket connection: the websocket is closed")
		http.Error(rw, "the service is shutting down", http.StatusServiceUnavailable)

		return
	}

	conn, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		w.logger.Error("failed to upgrade websocket connection", "error", err)

		return
	}

	w.logger.Info("new client connected", "remoteAddress", conn.RemoteAddr().String())

	w.router.NewSession(transport.NewConnection(conn, w.config.connection(), w.logger)).Run(r.Context())
}

// Close stops carrying replies and releases the subscription that brought them
// in. Open connections stay up, but the requests they read are refused.
func (w *Websocket) Close() error {
	return w.bus.Shutdown()
}

// fanoutReplies hands every reply this replica receives to all of its sessions.
func (w *Websocket) fanoutReplies() {
	for {
		select {
		case <-w.bus.Closed():
			return

		case reply := <-w.bus.Receive():
			w.logger.Info("publishing reply to all response channels", "requestID", reply.RequestID)

			w.hub.Broadcast(reply)
		}
	}
}
