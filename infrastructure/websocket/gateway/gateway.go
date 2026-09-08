package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

var (
	// ErrClosed is returned by a gateway that has been shut down: it can no
	// longer take a client, nor carry a reply to one.
	ErrClosed = errors.New("the gateway is closed")

	// ErrRequestIDRequired is returned for a reply that names no request.
	ErrRequestIDRequired = errors.New("request id is required")
)

// Gateway assembles the path a request takes and the path its reply takes back.
// It performs none of those steps itself: it wires the pieces together and
// gives every connection a session of its own.
type Gateway struct {
	// a ProduceConsumer is two roles: the gateway consumes the subjects clients
	// may send to, while a session's dispatcher produces onto them.
	consumer domain.Consumer
	producer domain.Producer

	// cancellations travel the same way replies do: published to every
	// replica, because the client that walked away and the handler producing
	// its stream need not be on the same one.
	publishSubscriber domain.PublishSubscriber

	subjects   *subjects
	hub        *hub
	bus        *replyBus
	translator translator.Translator
	config     configuration
	logger     *slog.Logger
}

var (
	// a gateway consumes the subjects its clients may send requests to.
	_ domain.Consumer = &Gateway{}

	// a gateway carries replies back to whichever replica holds the client.
	_ domain.Replyer = &Gateway{}

	// a gateway serves the clients a transport hands it.
	_ Sessions = &Gateway{}

	// closing a gateway releases the reply path and every client on it.
	_ io.Closer = &Gateway{}
)

// New builds a gateway that produces requests onto produceConsumer and carries
// replies over publishSubscriber. The replies subject is shared by every
// replica, so all of them see every reply and the one holding the client
// delivers it.
func New(
	produceConsumer domain.ProduceConsumer,
	publishSubscriber domain.PublishSubscriber,
	translator translator.Translator,
	repliesSubject string,
	logger *slog.Logger,
	options ...Option,
) (*Gateway, error) {
	config, err := newConfiguration(options...)
	if err != nil {
		return nil, err
	}

	g := &Gateway{
		consumer:          produceConsumer,
		producer:          produceConsumer,
		publishSubscriber: publishSubscriber,

		subjects:   newSubjects(),
		hub:        newHub(logger),
		bus:        newReplyBus(publishSubscriber, config.subjectPrefix+repliesSubject, logger),
		translator: translator,
		config:     config,
		logger:     logger,
	}

	if err := g.bus.start(context.Background()); err != nil {
		return nil, err
	}

	go g.fanoutReplies()

	return g, nil
}

// Consume subscribes to the given subject and handles incoming messages with
// the given handler once for each message, however many replicas are running.
// Consuming a subject is also what allows clients to send requests to it.
func (g *Gateway) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	if err := g.consumer.Consume(ctx, g.config.subjectPrefix+subject, handler); err != nil {
		return err
	}

	g.subjects.add(subject)

	return nil
}

// WatchStreamCancellations tells handler about the streams whose clients have
// stopped listening, so that whatever is producing one can stop. A *Streams is
// the handler to pass here.
func (g *Gateway) WatchStreamCancellations(ctx context.Context, handler domain.MessageHandler) error {
	return g.publishSubscriber.Subscribe(ctx, g.config.subjectPrefix+cancellationsSubject, handler)
}

// cancelStream announces that nobody is listening to a stream any more. It is
// best effort: a cancellation that cannot be published costs a stream that runs
// until its own source ends, not a broken client.
func (g *Gateway) cancelStream(ctx context.Context, requestID string) {
	payload, err := json.Marshal(&StreamCancelled{RequestID: requestID})
	if err != nil {
		g.logger.Error("error on marshalling a stream cancellation", "error", err, "requestID", requestID)

		return
	}

	if err := g.publishSubscriber.Publish(ctx, g.config.subjectPrefix+cancellationsSubject, payload); err != nil {
		g.logger.Error("error on publishing a stream cancellation", "error", err, "requestID", requestID)
	}
}

// Reply hands a reply to every replica, because the client waiting for it may
// be connected to any of them.
func (g *Gateway) Reply(ctx context.Context, reply *domain.Reply) error {
	if g.bus.isClosed() {
		return ErrClosed
	}

	if len(reply.RequestID) == 0 {
		return ErrRequestIDRequired
	}

	return g.bus.publish(ctx, reply)
}

// Accept refuses new clients once the gateway is closed: nothing could carry a
// reply back to them, so serving one would produce work whose answer it could
// never receive.
func (g *Gateway) Accept() error {
	if g.bus.isClosed() {
		return ErrClosed
	}

	return nil
}

// Serve drives one client's conversation and closes the connection when it ends.
func (g *Gateway) Serve(ctx context.Context, conn Conn) {
	g.newSession(conn).run(ctx)
}

// Close stops the gateway: the reply path is released and every client it is
// serving is disconnected. It returns once those clients are gone, so a caller
// that closes a gateway is left holding nothing.
func (g *Gateway) Close() error {
	// shut the reply path first, so no reply is fanned out to a session that is
	// about to be told to stop.
	err := g.bus.shutdown()

	g.hub.closeAll()

	return err
}

// ConnectedClients reports how many clients this replica is currently serving.
func (g *Gateway) ConnectedClients() int {
	return g.hub.size()
}

// newSession gives one connection everything that client's conversation needs:
// a request registry of its own, the validator and dispatcher that hold it, and
// its own queue of incoming replies.
//
// The registry is per connection, so the request ids a client picks are private
// to it, two clients may use the same ones at the same time, and a session can
// only ever resolve a reply to a request it registered itself.
func (g *Gateway) newSession(conn Conn) *session {
	requests := g.config.registries()

	return &session{
		conn: conn,
		dispatcher: &dispatcher{
			validator:     newRequestValidator(requests, g.subjects, g.translator, g.config.maxInFlightRequests),
			registry:      requests,
			producer:      g.producer,
			subjectPrefix: g.config.subjectPrefix,
			logger:        g.logger,
		},
		registry:     requests,
		cancelStream: g.cancelStream,
		hub:          g.hub,
		bus:          g.bus,
		replies:      make(chan *domain.Reply, g.config.replyBuffer),
		done:         make(chan struct{}),
		gone:         make(chan struct{}),
		replyBackoff: g.config.replyBackoff,
		queueBackoff: g.config.queueBackoff,
		translator:   g.translator,
		logger:       g.logger,
	}
}

// fanoutReplies hands every reply this replica receives to all of its sessions.
func (g *Gateway) fanoutReplies() {
	for {
		select {
		case <-g.bus.closed():
			return

		case reply := <-g.bus.receive():
			g.logger.Info("publishing reply to all sessions", "requestID", reply.RequestID)

			g.hub.broadcast(reply)
		}
	}
}
