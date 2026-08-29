package transport

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

var (
	// ErrClosed is returned when replying on a websocket that is already closed.
	ErrClosed = errors.New("connection is closed")

	// ErrRequestIDRequired is returned for a reply that names no request.
	ErrRequestIDRequired = errors.New("request id is required")
)

// ReplyBus carries replies between replicas. A client is connected to one
// replica, but the reply to its request may be produced on any of them.
type ReplyBus struct {
	publishSubscriber domain.PublishSubscriber
	subject           string
	replies           chan *domain.Reply
	done              chan struct{}
	unsubscribe       context.CancelFunc
	close             sync.Once
	logger            *slog.Logger
}

func NewReplyBus(publishSubscriber domain.PublishSubscriber, subject string, logger *slog.Logger) *ReplyBus {
	return &ReplyBus{
		publishSubscriber: publishSubscriber,
		subject:           subject,
		replies:           make(chan *domain.Reply),
		done:              make(chan struct{}),
		logger:            logger,
	}
}

// start subscribes to the replies subject, which every replica receives.
func (b *ReplyBus) Start(ctx context.Context) error {
	ctx, b.unsubscribe = context.WithCancel(ctx)

	err := b.publishSubscriber.Subscribe(
		ctx,
		b.subject,
		domain.MessageHandlerFunc(func(ctx context.Context, payload []byte) error {
			var reply domain.Reply

			if err := json.Unmarshal(payload, &reply); err != nil {
				b.logger.Error("error on unmarshalling reply", "error", err)

				// a malformed reply is not worth redelivering.
				return nil
			}

			b.logger.Info("reply received", "reply", reply)

			b.push(&reply)

			return nil
		}),
	)
	if err != nil {
		b.unsubscribe()
	}

	return err
}

// publish hands a reply to every replica, this one included.
func (b *ReplyBus) Publish(ctx context.Context, reply *domain.Reply) error {
	payload, err := json.Marshal(reply)
	if err != nil {
		return err
	}

	return b.publishSubscriber.Publish(ctx, b.subject, payload)
}

// receive is the stream of replies this replica has to deliver.
func (b *ReplyBus) Receive() <-chan *domain.Reply {
	return b.replies
}

// push hands a reply to the local stream. It blocks until the stream takes it,
// and reports false once the bus is closed.
func (b *ReplyBus) push(reply *domain.Reply) bool {
	select {
	case b.replies <- reply:
		return true
	case <-b.done:
		return false
	}
}

// closed returns a channel that is closed when the bus shuts down.
func (b *ReplyBus) Closed() <-chan struct{} {
	return b.done
}

func (b *ReplyBus) IsClosed() bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}

// shutdown stops the bus. It is safe to call more than once.
func (b *ReplyBus) Shutdown() error {
	b.close.Do(func() {
		close(b.done)

		if b.unsubscribe != nil {
			b.unsubscribe()
		}
	})

	return nil
}
