package websocket

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

// replyBus carries replies between replicas. A client is connected to one
// replica, but the reply to its request may be produced on any of them.
type replyBus struct {
	publishSubscriber domain.PublishSubscriber
	subject           string
	replies           chan *domain.Reply
	done              chan struct{}
	close             sync.Once
	logger            *slog.Logger
}

func newReplyBus(publishSubscriber domain.PublishSubscriber, subject string, logger *slog.Logger) *replyBus {
	return &replyBus{
		publishSubscriber: publishSubscriber,
		subject:           subject,
		replies:           make(chan *domain.Reply),
		done:              make(chan struct{}),
		logger:            logger,
	}
}

// start subscribes to the replies subject, which every replica receives.
func (b *replyBus) start(ctx context.Context) error {
	return b.publishSubscriber.Subscribe(
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
}

// publish hands a reply to every replica, this one included.
func (b *replyBus) publish(ctx context.Context, reply *domain.Reply) error {
	payload, err := json.Marshal(reply)
	if err != nil {
		return err
	}

	return b.publishSubscriber.Publish(ctx, b.subject, payload)
}

// receive is the stream of replies this replica has to deliver.
func (b *replyBus) receive() <-chan *domain.Reply {
	return b.replies
}

// push hands a reply to the local stream. It blocks until the stream takes it,
// and reports false once the bus is closed.
func (b *replyBus) push(reply *domain.Reply) bool {
	select {
	case b.replies <- reply:
		return true
	case <-b.done:
		return false
	}
}

// closed returns a channel that is closed when the bus shuts down.
func (b *replyBus) closed() <-chan struct{} {
	return b.done
}

func (b *replyBus) isClosed() bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}

// shutdown stops the bus. It is safe to call more than once.
func (b *replyBus) shutdown() error {
	b.close.Do(func() {
		close(b.done)
	})

	return nil
}
