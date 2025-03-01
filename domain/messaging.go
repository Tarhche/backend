package domain

import (
	"context"
)

// Handler handlers a message with given payload
type MessageHandler interface {
	Handle(payload []byte) error
}

type MessageHandlerFunc func(payload []byte) error

func (handler MessageHandlerFunc) Handle(payload []byte) error {
	return handler(payload)
}

type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

type Subscriber interface {
	Subscribe(ctx context.Context, consumerID string, subject string, subscriber MessageHandler) error
}

type PublishSubscriber interface {
	Publisher
	Subscriber
}
