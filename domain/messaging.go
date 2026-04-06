package domain

import (
	"context"
)

// Request reflects a request from the client to the server
type Request struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Payload []byte `json:"payload"`
}

// Reply reflects a reply from the server to the client
type Reply struct {
	RequestID string `json:"request_id"`
	Payload   []byte `json:"payload"`
}

type Replyer interface {
	Reply(ctx context.Context, reply *Reply) error
}

type RequestRegistry interface {
	Add(clientSideID string) (string, error)
	GetClientSideID(serverSideID string) (string, error)
	GetServerSideID(clientSideID string) (string, error)
	DeleteByServerSideID(serverSideID string) error
}

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
	Subscribe(ctx context.Context, subject string, handler MessageHandler) error
}

type PublishSubscriber interface {
	Publisher
	Subscriber
}

type Producer interface {
	Produce(ctx context.Context, subject string, payload []byte) error
}

type Consumer interface {
	Consume(ctx context.Context, subject string, handler MessageHandler) error
}

type ProduceConsumer interface {
	Producer
	Consumer
}
