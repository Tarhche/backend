package pubsub

import (
	"context"
	"log"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"

	"github.com/nats-io/nats.go"
)

type publishSubscriber struct {
	connection *nats.Conn
	wg         sync.WaitGroup
}

var _ domain.PublishSubscriber = &publishSubscriber{}

func NewPublishSubscriber(connection *nats.Conn) *publishSubscriber {
	return &publishSubscriber{
		connection: connection,
	}
}

func (m *publishSubscriber) Publish(ctx context.Context, subject string, payload []byte) error {
	return m.connection.Publish(subject, payload)
}

func (m *publishSubscriber) Subscribe(ctx context.Context, subject string, handler domain.MessageHandler) error {
	m.wg.Add(1)

	sub, err := m.connection.Subscribe(subject, func(msg *nats.Msg) {
		if err := handler.Handle(msg.Data); err != nil {
			log.Println("consume error", err, msg.Subject)
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
