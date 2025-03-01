package memory

import (
	"context"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

type bus struct {
	lock        sync.RWMutex
	subscribers map[string][]subscriber
}

type subscriber struct {
	id      string
	handler domain.MessageHandler
}

var _ domain.PublishSubscriber = NewSyncPublishSubscriber()

func NewSyncPublishSubscriber() *bus {
	return &bus{
		subscribers: make(map[string][]subscriber),
	}
}

func (m *bus) Subscribe(ctx context.Context, consumerID string, subject string, handler domain.MessageHandler) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.subscribers[subject] = append(m.subscribers[subject], subscriber{
		id:      consumerID,
		handler: handler,
	})

	return nil
}

func (m *bus) Publish(ctx context.Context, subject string, payload []byte) error {
	m.lock.RLock()
	defer m.lock.RUnlock()

	for i := range m.subscribers[subject] {
		if err := m.subscribers[subject][i].handler.Handle(payload); err != nil {
			return err
		}
	}

	return nil
}
