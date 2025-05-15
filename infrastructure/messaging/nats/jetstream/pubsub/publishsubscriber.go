package pubsub

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type publishSubscriber struct {
	connection *nats.Conn
	jetstream  jetstream.JetStream
	streams    map[string]jetstream.Stream
	lock       sync.RWMutex
	wg         sync.WaitGroup
}

type subscriber struct {
	id      string
	handler domain.MessageHandler
	stream  jetstream.Stream
}

var _ domain.PublishSubscriber = &publishSubscriber{}

func NewPublishSubscriber(connection *nats.Conn) (*publishSubscriber, error) {
	j, err := jetstream.New(connection)
	if err != nil {
		return nil, err
	}

	s := &publishSubscriber{
		connection: connection,
		jetstream:  j,
		streams:    make(map[string]jetstream.Stream),
	}

	return s, nil
}

func (m *publishSubscriber) Publish(ctx context.Context, subject string, payload []byte) error {
	if _, err := m.makeSureStreamExists(ctx, subject); err != nil {
		return err
	}

	_, err := m.jetstream.Publish(ctx, subject, payload)

	return err
}

func (m *publishSubscriber) Subscribe(ctx context.Context, consumerID string, subject string, handler domain.MessageHandler) error {
	stream, err := m.makeSureStreamExists(ctx, subject)
	if err != nil {
		return err
	}

	return m.consumer(ctx, &subscriber{
		id:      consumerID,
		handler: handler,
		stream:  stream,
	})
}

func (m *publishSubscriber) Wait() {
	m.wg.Wait()
}

func (m *publishSubscriber) consumer(ctx context.Context, subscriber *subscriber) error {
	m.wg.Add(1)

	consumer, err := subscriber.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:      subscriber.id,
		Durable:   subscriber.id,
		AckPolicy: jetstream.AckExplicitPolicy,
		AckWait:   30 * time.Second,
	})
	if err != nil {
		m.wg.Done()
		return err
	}

	c, err := consumer.Consume(m.consume(subscriber.handler))
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

func (m *publishSubscriber) consume(handler domain.MessageHandler) func(msg jetstream.Msg) {
	return func(msg jetstream.Msg) {
		if err := msg.InProgress(); err != nil {
			log.Println("in progress error", err)
		}

		if err := handler.Handle(msg.Data()); err != nil {
			log.Println("consume error", err, string(msg.Subject()))

			if err := msg.Nak(); err != nil {
				log.Println("nak error", err)
			}
			return
		}

		if err := msg.DoubleAck(context.Background()); err != nil {
			log.Println("double ack error", err)
		}
	}
}

func (m *publishSubscriber) makeSureStreamExists(ctx context.Context, subject string) (jetstream.Stream, error) {
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
