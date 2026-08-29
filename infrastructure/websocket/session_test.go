package websocket

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// fakeConn is an in-memory transport, so a session can be driven without a
// socket, an upgrade handshake or a port.
type fakeConn struct {
	incoming chan domain.Request

	lock     sync.Mutex
	outgoing []*domain.Reply
	closed   bool
}

var _ conn = &fakeConn{}

func newFakeConn(requests ...domain.Request) *fakeConn {
	incoming := make(chan domain.Request, len(requests))
	for _, request := range requests {
		incoming <- request
	}
	close(incoming)

	return &fakeConn{incoming: incoming}
}

// read drains the queued requests, then reports the peer as gone.
func (c *fakeConn) read(value any) error {
	request, ok := <-c.incoming
	if !ok {
		return io.EOF
	}

	*(value.(*domain.Request)) = request

	return nil
}

func (c *fakeConn) send(value any) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.closed {
		return false
	}

	c.outgoing = append(c.outgoing, value.(*domain.Reply))

	return true
}

func (c *fakeConn) shutdown() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.closed = true

	return nil
}

func (c *fakeConn) written() []*domain.Reply {
	c.lock.Lock()
	defer c.lock.Unlock()

	return append([]*domain.Reply(nil), c.outgoing...)
}

func TestSession(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	newTestSession := func(c conn, registry domain.RequestRegistry, producer domain.Producer, tr *translator.TranslatorMock, consumedSubject string) *session {
		subjects := newSubjects()
		subjects.add(consumedSubject)

		return &session{
			conn: c,
			dispatcher: &dispatcher{
				validator: newRequestValidator(registry, subjects, tr),
				registry:  registry,
				producer:  producer,
				logger:    logger,
			},
			registry:   registry,
			hub:        newHub(defaultOutboundBuffer, logger),
			bus:        newReplyBus(&messagingMock.MockPublishSubscriber{}, "replies", logger),
			translator: tr,
			logger:     logger,
		}
	}

	t.Run("dispatches a valid request onto the queue", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		registryMock.On("Add", "req-1").Return("server-1", nil)
		// the client leaves without waiting for a reply, so the entry is swept.
		registryMock.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registryMock.AssertExpectations(t)

		var producerMock messagingMock.MockProduceConsumer
		producerMock.On("Produce", mock.Anything, "websocket_runCode", mock.MatchedBy(func(payload []byte) bool {
			var data map[string]any
			assert.NoError(t, json.Unmarshal(payload, &data))

			return data["id"] == "server-1" && data["code"] == "print(1)"
		})).Return(nil).Once()
		defer producerMock.AssertExpectations(t)

		translatorMock := echoTranslator()

		c := newFakeConn(domain.Request{ID: "req-1", Subject: "runCode", Payload: []byte(`{"code":"print(1)"}`)})

		newTestSession(c, &registryMock, &producerMock, translatorMock, "runCode").run(context.Background())

		assert.Empty(t, c.written(), "a dispatched request is answered by the queue, not by the session")
	})

	t.Run("answers a rejected request without touching the queue", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		defer registryMock.AssertExpectations(t)

		var producerMock messagingMock.MockProduceConsumer
		defer producerMock.AssertExpectations(t)

		translatorMock := echoTranslator()

		c := newFakeConn(domain.Request{Subject: "nobody-listens-here"})

		newTestSession(c, &registryMock, &producerMock, translatorMock, "runCode").run(context.Background())

		written := c.written()
		assert.Len(t, written, 1)
		assert.JSONEq(
			t,
			`{"validationErrors":{"request_id":"required_field","subject":"invalid_value"}}`,
			string(written[0].Payload),
		)
		producerMock.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("writes back the reply under the id the client chose", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetClientSideID", "server-1").Return("client-1", nil).Once()
		registryMock.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")

		replies := make(chan *domain.Reply, 1)
		replies <- &domain.Reply{RequestID: "server-1", Payload: []byte("done")}
		close(replies)

		s.writeReplies(replies)

		written := c.written()
		assert.Len(t, written, 1)
		assert.Equal(t, "client-1", written[0].RequestID)
		assert.Equal(t, []byte("done"), written[0].Payload)
	})

	t.Run("leaves a reply it does not own to the session that does", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetClientSideID", "someone-elses-request").Return("", domain.ErrNotExists).Once()
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")

		replies := make(chan *domain.Reply, 1)
		replies <- &domain.Reply{RequestID: "someone-elses-request"}
		close(replies)

		done := make(chan struct{})
		go func() {
			defer close(done)

			s.writeReplies(replies)
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("writeReplies blocked on a reply belonging to another session")
		}

		assert.Empty(t, c.written())
		registryMock.AssertNotCalled(t, "DeleteByServerSideID", "someone-elses-request")
	})
}
