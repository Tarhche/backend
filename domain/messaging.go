package domain

import (
	"context"
	"errors"
)

// publish/subscribe interfaces
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

// request/reply interfaces
var ErrReplierNotFound = errors.New("replier not found")

type Replyer interface {
	Reply(request Request, replyChan chan<- *Reply) error
}

type Request struct {
	ID      string
	Subject string
	Payload []byte
}

type Reply struct {
	RequestID string
	Payload   []byte
}

type Requester interface {
	Request(ctx context.Context, request *Request) error
	RegisterReplyer(ctx context.Context, subject string, replyer Replyer) error
}
