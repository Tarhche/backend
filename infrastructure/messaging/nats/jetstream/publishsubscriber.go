package jetstream

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

	go m.consumer(ctx, &subscriber{
		id:      consumerID,
		handler: handler,
		stream:  stream,
	})

	return nil
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
		return err
	}

	c, err := consumer.Consume(m.consume(subscriber.handler))
	if err != nil {
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
		_ = msg.InProgress()
		log.Printf("message recieved: %s\n", msg.Subject())
		if err := handler.Handle(msg.Data()); err != nil {
			_ = msg.Nak()
			log.Printf("message Nak: %s | error: %s\n", msg.Subject(), err.Error())
			return
		}
		_ = msg.DoubleAck(context.Background())
		log.Printf("message Ack: %s\n", msg.Subject())
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
		Name:     subject,
		Subjects: []string{subject},
	})
	if err != nil {
		return nil, err
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	m.streams[subject] = stream

	return stream, nil
}
