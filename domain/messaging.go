package domain

import (
	"context"
)

// Request reflects a request from the client to the server
type Request struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`

	// StreamID names an already-open request this one acts on rather than a new
	// question of its own. With a subject it carries input to that stream —
	// keystrokes for an attached terminal, say — and without one it asks for the
	// stream to end. Either way it registers nothing and is not replied to, so a
	// client can talk to a stream as often as it likes.
	StreamID string `json:"stream_id,omitempty"`

	Payload []byte `json:"payload"`
}

// ReplyKind tells a client whether a reply is the whole answer or one piece of
// a longer one.
type ReplyKind uint8

const (
	// ReplyFinal is the whole answer to a request; nothing more follows. It is
	// the zero value, so anything replying once needs to say nothing at all.
	ReplyFinal ReplyKind = 0

	// ReplyChunk is one piece of a stream. More will follow, so the request
	// stays open.
	ReplyChunk ReplyKind = 1

	// ReplyEOF ends a stream. It carries no payload of its own.
	ReplyEOF ReplyKind = 2
)

// EndsRequest reports whether nothing further will be sent for the request.
func (k ReplyKind) EndsRequest() bool {
	return k != ReplyChunk
}

// Reply reflects a reply from the server to the client
type Reply struct {
	RequestID string    `json:"request_id"`
	Kind      ReplyKind `json:"kind,omitempty"`
	Payload   []byte    `json:"payload"`
}

type Replyer interface {
	Reply(ctx context.Context, reply *Reply) error
}

// publish/subscribe interfaces
type MessageHandler interface {
	Handle(ctx context.Context, payload []byte) error
}

type MessageHandlerFunc func(ctx context.Context, payload []byte) error

func (handler MessageHandlerFunc) Handle(ctx context.Context, payload []byte) error {
	return handler(ctx, payload)
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
