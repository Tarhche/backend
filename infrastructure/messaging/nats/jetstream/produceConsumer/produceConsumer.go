package produceConsumer

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	infranats "github.com/khanzadimahdi/testproject/infrastructure/messaging/nats"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const defaultAckWait = 30 * time.Second

type produceConsumer struct {
	connection *nats.Conn
	jetstream  jetstream.JetStream
	consumerID string
	ackWait    time.Duration
	streams    map[string]jetstream.Stream
	lock       sync.RWMutex
	wg         sync.WaitGroup
	logger     *slog.Logger
	tracer     oteltrace.Tracer

	// concurrency is how many messages of one subject are handled at once,
	// and progress how often one being handled is said to still be.
	concurrency int
	progress    time.Duration
}

var _ domain.ProduceConsumer = &produceConsumer{}

type Option func(*produceConsumer)

// WithAckWait sets how long JetStream waits for an ack before redelivering a
// message; it must exceed the slowest handler, otherwise in-flight messages
// are redelivered and processed more than once.
func WithAckWait(d time.Duration) Option {
	return func(m *produceConsumer) {
		m.ackWait = d
	}
}

// WithConcurrency handles up to n messages of a subject at once rather than
// one after another.
//
// A message is acknowledged once its handler is done with it either way, so
// one that is redelivered after a crash is still handled. What changes is that
// a handler that takes minutes — a VM restored from a snapshot, a stack whose
// images are pulled — no longer holds up every message behind it on its
// subject. Handlers that run at once have to be safe to: what they do to the
// same thing has to take turns of its own accord.
func WithConcurrency(n int) Option {
	return func(m *produceConsumer) {
		m.concurrency = n
	}
}

// WithProgress tells JetStream every interval that a message being handled
// still is, so a handler that takes longer than the ack wait is not handed the
// same message again while it works. A handler that hangs holds its message for
// as long as it hangs, so this is for handlers that bound their own time.
func WithProgress(interval time.Duration) Option {
	return func(m *produceConsumer) {
		m.progress = interval
	}
}

func NewProduceConsumer(connection *nats.Conn, consumerID string, logger *slog.Logger, options ...Option) (*produceConsumer, error) {
	j, err := jetstream.New(connection)
	if err != nil {
		return nil, err
	}

	s := &produceConsumer{
		connection: connection,
		jetstream:  j,
		consumerID: consumerID,
		ackWait:    defaultAckWait,
		streams:    make(map[string]jetstream.Stream),
		logger:     logger,
		tracer:     otel.Tracer("nats.jetstream"),
	}

	for _, option := range options {
		option(s)
	}

	return s, nil
}

func (m *produceConsumer) Produce(ctx context.Context, subject string, payload []byte) error {
	ctx, span := m.tracer.Start(ctx, "jetstream.publish "+subject,
		oteltrace.WithSpanKind(oteltrace.SpanKindProducer),
		oteltrace.WithAttributes(attribute.String("messaging.destination.name", subject)),
	)
	defer span.End()

	if _, err := m.makeSureStreamExists(ctx, subject); err != nil {
		return trace.RecordError(span, err)
	}

	msg := &nats.Msg{Subject: subject, Data: payload, Header: nats.Header{}}
	otel.GetTextMapPropagator().Inject(ctx, infranats.HeaderCarrier(msg.Header))

	_, err := m.jetstream.PublishMsg(ctx, msg)

	return trace.RecordError(span, err)
}

func (m *produceConsumer) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	stream, err := m.makeSureStreamExists(ctx, subject)
	if err != nil {
		return err
	}

	return m.consumeInBackground(ctx, stream, handler)
}

func (m *produceConsumer) Wait() {
	m.wg.Wait()
}

func (m *produceConsumer) consumeInBackground(ctx context.Context, stream jetstream.Stream, handler domain.MessageHandler) error {
	m.wg.Add(1)

	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:      m.consumerID,
		Durable:   m.consumerID,
		AckPolicy: jetstream.AckExplicitPolicy,
		AckWait:   m.ackWait,
	})
	if err != nil {
		m.wg.Done()
		return err
	}

	c, err := consumer.Consume(m.consumeFunc(handler))
	if err != nil {
		m.wg.Done()
		return err
	}

	go func(c jetstream.ConsumeContext) {
		defer m.wg.Done()

		<-ctx.Done()
		c.Stop()
		<-c.Closed()
	}(c)

	return nil
}

func (m *produceConsumer) consumeFunc(handler domain.MessageHandler) func(msg jetstream.Msg) {
	if m.concurrency <= 1 {
		return func(msg jetstream.Msg) {
			m.handle(handler, msg)
		}
	}

	// a slot is taken before the next message is let in, so no more than the
	// concurrency are handled at once and the rest wait their turn in
	// JetStream rather than in memory.
	slots := make(chan struct{}, m.concurrency)

	return func(msg jetstream.Msg) {
		slots <- struct{}{}
		m.wg.Add(1)

		go func() {
			defer m.wg.Done()
			defer func() { <-slots }()

			m.handle(handler, msg)
		}()
	}
}

// handle handles one message, and acknowledges it once its handler is done
// with it, or asks for it again when its handler failed.
func (m *produceConsumer) handle(handler domain.MessageHandler, msg jetstream.Msg) {
	// the producer's span context arrives in the traceparent header; the
	// message is processed as a trace of its own that links back to the
	// originating trace instead of continuing it
	remoteCtx := otel.GetTextMapPropagator().Extract(context.Background(), infranats.HeaderCarrier(msg.Headers()))

	msgCtx, span := m.tracer.Start(context.Background(), "jetstream.consume "+msg.Subject(),
		oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
		oteltrace.WithLinks(oteltrace.LinkFromContext(remoteCtx)),
		oteltrace.WithAttributes(attribute.String("messaging.destination.name", msg.Subject())),
	)
	defer span.End()

	if err := msg.InProgress(); err != nil {
		m.logger.Error("in progress error", "error", err)
	}

	stopProgress := m.keepInProgress(msg)
	err := handler.Handle(msgCtx, msg.Data())
	stopProgress()

	if err := trace.RecordError(span, err); err != nil {
		m.logger.Error("consume error", "error", err, "subject", string(msg.Subject()))

		if err := msg.Nak(); err != nil {
			m.logger.Error("nak error", "error", err)
		}
		return
	}

	// Acking is a real infra call to NATS, not part of the traced unit of
	// work above - keep it on its own background context rather than the
	// (short-lived, span-scoped) message context.
	if err := msg.DoubleAck(context.Background()); err != nil {
		m.logger.Error("double ack error", "error", err)
	}
}

// keepInProgress tells JetStream every progress interval that msg is still
// being handled, until what it returns is called.
func (m *produceConsumer) keepInProgress(msg jetstream.Msg) func() {
	if m.progress <= 0 {
		return func() {}
	}

	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		ticker := time.NewTicker(m.progress)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := msg.InProgress(); err != nil {
					m.logger.Error("in progress error", "error", err)
				}
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}

func (m *produceConsumer) makeSureStreamExists(ctx context.Context, subject string) (jetstream.Stream, error) {
	m.lock.RLock()
	stream, ok := m.streams[subject]
	m.lock.RUnlock()
	if ok {
		return stream, nil
	}

	stream, err := m.jetstream.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      subject,
		Subjects:  []string{subject},
		Retention: jetstream.InterestPolicy,
	})
	if err != nil {
		return nil, err
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	m.streams[subject] = stream

	return stream, nil
}
