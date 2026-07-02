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

type produceConsumer struct {
	connection *nats.Conn
	jetstream  jetstream.JetStream
	consumerID string
	streams    map[string]jetstream.Stream
	lock       sync.RWMutex
	wg         sync.WaitGroup
	logger     *slog.Logger
	tracer     oteltrace.Tracer
}

var _ domain.ProduceConsumer = &produceConsumer{}

func NewProduceConsumer(connection *nats.Conn, consumerID string, logger *slog.Logger) (*produceConsumer, error) {
	j, err := jetstream.New(connection)
	if err != nil {
		return nil, err
	}

	s := &produceConsumer{
		connection: connection,
		jetstream:  j,
		consumerID: consumerID,
		streams:    make(map[string]jetstream.Stream),
		logger:     logger,
		tracer:     otel.Tracer("nats.jetstream"),
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
		AckWait:   30 * time.Second,
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
	return func(msg jetstream.Msg) {
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

		if err := trace.RecordError(span, handler.Handle(msgCtx, msg.Data())); err != nil {
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
