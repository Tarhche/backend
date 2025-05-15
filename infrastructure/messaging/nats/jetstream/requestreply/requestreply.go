package requestreply

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type subscriber struct {
	id      string
	replyer domain.Replyer
	stream  jetstream.Stream
}

// requestReply handles request/reply messaging
type requestReply struct {
	connection *nats.Conn
	jetstream  jetstream.JetStream
	streams    map[string]jetstream.Stream
	consumerID string

	replyChan chan *domain.Reply
	lock      sync.RWMutex
	wg        sync.WaitGroup
}

var _ domain.Requester = &requestReply{}

// New creates a new instance of requestReply
func New(connection *nats.Conn, consumerID string) (*requestReply, chan *domain.Reply, error) {
	j, err := jetstream.New(connection)
	if err != nil {
		return nil, nil, err
	}

	router := &requestReply{
		connection: connection,
		jetstream:  j,
		streams:    make(map[string]jetstream.Stream),
		consumerID: consumerID,
		replyChan:  make(chan *domain.Reply),
	}

	// TODO: make replyChan unidirectional recieve only (<-chan)
	return router, router.replyChan, nil
}

// RegisterReplyer registers a replyer for a specific topic
func (r *requestReply) RegisterReplyer(ctx context.Context, subject string, replyer domain.Replyer) error {
	stream, err := r.makeSureStreamExists(ctx, subject)
	if err != nil {
		return err
	}

	return r.consumer(ctx, &subscriber{
		id:      r.consumerID,
		replyer: replyer,
		stream:  stream,
	})
}

func (r *requestReply) Request(ctx context.Context, request *domain.Request) error {
	if !r.isStreamExists(request.Subject) {
		return domain.ErrReplierNotFound
	}

	_, err := r.jetstream.Publish(
		ctx,
		request.Subject,
		request.Payload,
		jetstream.WithMsgID(request.ID),
	)

	return err
}

// Close handles gracefully shutting down the router
func (r *requestReply) Close() {
	r.wg.Wait()
	close(r.replyChan)
}

// Wait waits for the consumers to finish their work
func (r *requestReply) Wait() {
	r.wg.Wait()
}

func (r *requestReply) consumer(ctx context.Context, subscriber *subscriber) error {
	r.wg.Add(1)

	consumer, err := subscriber.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:      subscriber.id,
		Durable:   subscriber.id,
		AckPolicy: jetstream.AckExplicitPolicy,
		AckWait:   30 * time.Second,
	})
	if err != nil {
		r.wg.Done()
		return err
	}

	c, err := consumer.Consume(r.consume(subscriber.replyer, r.replyChan))
	if err != nil {
		r.wg.Done()
		return err
	}

	go func(c jetstream.ConsumeContext) {
		defer r.wg.Done()

		<-ctx.Done()
		c.Stop()
		<-c.Closed()
	}(c)

	return nil
}

func (r *requestReply) consume(replyer domain.Replyer, responseChan chan<- *domain.Reply) func(msg jetstream.Msg) {
	return func(msg jetstream.Msg) {
		if err := msg.InProgress(); err != nil {
			log.Println("in progress error", err)
		}

		msgID := msg.Headers().Get(jetstream.MsgIDHeader)

		if err := replyer.Reply(
			domain.Request{
				ID:      msgID,
				Subject: string(msg.Subject()),
				Payload: msg.Data(),
			},
			responseChan,
		); err != nil {
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

func (r *requestReply) isStreamExists(subject string) bool {
	r.lock.RLock()
	defer r.lock.RUnlock()

	_, ok := r.streams[subject]

	return ok
}

func (r *requestReply) makeSureStreamExists(ctx context.Context, subject string) (jetstream.Stream, error) {
	r.lock.RLock()
	stream, ok := r.streams[subject]
	r.lock.RUnlock()
	if ok {
		return stream, nil
	}

	stream, err := r.jetstream.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      subject,
		Subjects:  []string{subject},
		Retention: jetstream.InterestPolicy,
	})
	if err != nil {
		return nil, err
	}

	r.lock.Lock()
	defer r.lock.Unlock()
	r.streams[subject] = stream

	return stream, nil
}
