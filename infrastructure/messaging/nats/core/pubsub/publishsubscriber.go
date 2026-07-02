package pubsub

import (
	"context"
	"log/slog"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
	infranats "github.com/khanzadimahdi/testproject/infrastructure/messaging/nats"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type publishSubscriber struct {
	connection *nats.Conn
	wg         sync.WaitGroup
	logger     *slog.Logger
	tracer     oteltrace.Tracer
}

var _ domain.PublishSubscriber = &publishSubscriber{}

func NewPublishSubscriber(connection *nats.Conn, logger *slog.Logger) *publishSubscriber {
	return &publishSubscriber{
		connection: connection,
		logger:     logger,
		tracer:     otel.Tracer("nats"),
	}
}

func (m *publishSubscriber) Publish(ctx context.Context, subject string, payload []byte) error {
	ctx, span := m.tracer.Start(ctx, "nats.publish "+subject,
		oteltrace.WithSpanKind(oteltrace.SpanKindProducer),
		oteltrace.WithAttributes(attribute.String("messaging.destination.name", subject)),
	)
	defer span.End()

	msg := &nats.Msg{Subject: subject, Data: payload, Header: nats.Header{}}
	otel.GetTextMapPropagator().Inject(ctx, infranats.HeaderCarrier(msg.Header))

	return trace.RecordError(span, m.connection.PublishMsg(msg))
}

func (m *publishSubscriber) Subscribe(ctx context.Context, subject string, handler domain.MessageHandler) error {
	m.wg.Add(1)

	sub, err := m.connection.Subscribe(subject, func(msg *nats.Msg) {
		// the producer's span context arrives in the traceparent header; the
		// message is processed as a trace of its own that links back to the
		// originating trace instead of continuing it
		remoteCtx := otel.GetTextMapPropagator().Extract(context.Background(), infranats.HeaderCarrier(msg.Header))

		msgCtx, span := m.tracer.Start(context.Background(), "nats.consume "+msg.Subject,
			oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
			oteltrace.WithLinks(oteltrace.LinkFromContext(remoteCtx)),
			oteltrace.WithAttributes(attribute.String("messaging.destination.name", msg.Subject)),
		)
		defer span.End()

		if err := trace.RecordError(span, handler.Handle(msgCtx, msg.Data)); err != nil {
			m.logger.Error("consume error", "error", err, "subject", msg.Subject)
		}
	})
	if err != nil {
		m.wg.Done()
		return err
	}

	go func() {
		defer m.wg.Done()
		<-ctx.Done()
		_ = sub.Drain()
	}()

	return nil
}

func (m *publishSubscriber) Wait() {
	m.wg.Wait()
}
