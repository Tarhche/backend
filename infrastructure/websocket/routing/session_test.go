package routing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/protocol"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/transport"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/websockettest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// echoTranslator returns every message key unchanged.
func echoTranslator() *translator.TranslatorMock {
	translatorMock := &translator.TranslatorMock{}

	for _, key := range []string{
		protocol.RequiredFieldMessage,
		protocol.InvalidValueMessage,
		protocol.RequestAlreadyExistsMessage,
		protocol.ErrorOnProcessingMessage,
	} {
		translatorMock.On("Translate", key, mock.AnythingOfType("[]func(*translator.Params)")).Return(key).Maybe()
	}

	return translatorMock
}

// fakeConn is an in-memory transport, so a Session can be driven without a
// socket, an upgrade handshake or a port.
type fakeConn struct {
	incoming chan domain.Request

	lock     sync.Mutex
	outgoing []*domain.Reply
	closed   bool

	// refuse makes send report a full queue, the way a client that is not
	// keeping up does.
	refuse bool
}

var _ Conn = &fakeConn{}

func newFakeConn(requests ...domain.Request) *fakeConn {
	incoming := make(chan domain.Request, len(requests))
	for _, request := range requests {
		incoming <- request
	}
	close(incoming)

	return &fakeConn{incoming: incoming}
}

// read drains the queued requests, then reports the peer as gone.
func (c *fakeConn) Read(value any) error {
	request, ok := <-c.incoming
	if !ok {
		return io.EOF
	}

	*(value.(*domain.Request)) = request

	return nil
}

func (c *fakeConn) Send(value any) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.closed || c.refuse {
		return false
	}

	c.outgoing = append(c.outgoing, value.(*domain.Reply))

	return true
}

func (c *fakeConn) Shutdown() error {
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

// panickingConn panics on read, standing in for a panic raised anywhere inside
// the read loop: a validation rule, the producer, the registry.
type panickingConn struct {
	*fakeConn
}

func (c *panickingConn) Read(value any) error {
	panic("boom")
}

// panickingRegistry panics on lookup, standing in for a panic raised anywhere
// inside the reply path.
type panickingRegistry struct {
	RequestRegistry
}

func (panickingRegistry) GetClientSideID(string) (string, error) {
	panic("boom")
}

// testOutboundBuffer matches the shipped default, which is what the delivery
// tests are measured against.
const testOutboundBuffer = 10

func TestSession(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	newTestSession := func(c Conn, registry RequestRegistry, producer domain.Producer, tr *translator.TranslatorMock, consumedSubject string) *Session {
		subjects := protocol.NewSubjects()
		subjects.Add(consumedSubject)

		router := &Router{
			Producer:   producer,
			Subjects:   subjects,
			Hub:        transport.NewHub(testOutboundBuffer, logger),
			Bus:        transport.NewReplyBus(&messagingMock.MockPublishSubscriber{}, "replies", logger),
			Backoffs:   Backoffs{Reply: NewFixedBackoff(defaultReplyAttempts, 0), Queue: NewFixedBackoff(defaultQueueAttempts, 0)},
			Registries: func() RequestRegistry { return registry },
			Translator: tr,
			Logger:     logger,
		}

		return router.NewSession(c)
	}

	t.Run("dispatches a valid request onto the queue", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry
		registryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		registryMock.On("Add", "req-1").Return("server-1", nil)
		defer registryMock.AssertExpectations(t)

		var producerMock messagingMock.MockProduceConsumer
		producerMock.On("Produce", mock.Anything, protocol.BrokerSubject("runCode"), mock.MatchedBy(func(payload []byte) bool {
			var data map[string]any
			assert.NoError(t, json.Unmarshal(payload, &data))

			return data["id"] == "server-1" && data["code"] == "print(1)"
		})).Return(nil).Once()
		defer producerMock.AssertExpectations(t)

		translatorMock := echoTranslator()

		c := newFakeConn(domain.Request{ID: "req-1", Subject: "runCode", Payload: []byte(`{"code":"print(1)"}`)})

		newTestSession(c, &registryMock, &producerMock, translatorMock, "runCode").Run(context.Background())

		assert.Empty(t, c.written(), "a dispatched request is answered by the queue, not by the session")
	})

	t.Run("answers a rejected request without touching the queue", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry
		defer registryMock.AssertExpectations(t)

		var producerMock messagingMock.MockProduceConsumer
		defer producerMock.AssertExpectations(t)

		translatorMock := echoTranslator()

		c := newFakeConn(domain.Request{Subject: "nobody-listens-here"})

		newTestSession(c, &registryMock, &producerMock, translatorMock, "runCode").Run(context.Background())

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

		var registryMock MockRegistry
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

	t.Run("unwinds cleanly when the read loop panics", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry

		s := newTestSession(
			&panickingConn{fakeConn: newFakeConn()},
			&registryMock,
			&messagingMock.MockProduceConsumer{},
			echoTranslator(),
			"runCode",
		)

		func() {
			// net/http recovers a panic per connection, so the process lives on
			// and anything the Session left behind lives on with it.
			defer func() { assert.NotNil(t, recover(), "expected the panic to reach the caller") }()

			s.Run(context.Background())
		}()

		assert.Equal(t, 0, s.hub.Size(), "the hub still holds this session's channel, so its writeReplies goroutine can never be woken")
	})

	t.Run("stops retrying as soon as the client disconnects", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry
		registryMock.On("GetClientSideID", "server-1").Return("", errors.New("registry is unreachable"))

		s := newTestSession(newFakeConn(), &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		// long enough that finishing quickly can only mean the disconnect was noticed
		s.replyBackoff = NewFixedBackoff(5, time.Minute)
		s.done = make(chan struct{})

		replies, unsubscribe := s.hub.Subscribe()

		written := make(chan struct{})
		go func() {
			defer close(written)

			s.writeReplies(replies)
		}()

		// a reply arrives that cannot be routed, so the Session starts backing off
		s.hub.Broadcast(&domain.Reply{RequestID: "server-1"})
		time.Sleep(20 * time.Millisecond)

		// the client goes away, which is what run does once readRequests returns
		start := time.Now()
		close(s.done)
		unsubscribe()
		<-written

		assert.Less(t, time.Since(start), time.Second, "teardown waited out a backoff for a client that had already gone")
	})

	t.Run("gives up on a reply after the backoff runs out", func(t *testing.T) {
		t.Parallel()

		unreachable := errors.New("registry is unreachable")

		var registryMock MockRegistry
		// three attempts, then the reply is dropped: a fourth call would fail
		// the mock, which is the assertion.
		registryMock.On("GetClientSideID", "server-1").Return("", unreachable).Times(3)
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		s.replyBackoff = NewFixedBackoff(3, 0)

		replies := make(chan *domain.Reply, 1)
		replies <- &domain.Reply{RequestID: "server-1"}
		close(replies)

		done := make(chan struct{})
		go func() {
			defer close(done)

			s.writeReplies(replies)
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("writeReplies never stopped retrying")
		}

		assert.Empty(t, c.written(), "an undeliverable reply should be dropped, not written to the wrong client")
		registryMock.AssertNotCalled(t, "DeleteByServerSideID", "server-1")
	})

	t.Run("keeps the request addressable when the client's queue is full", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry
		registryMock.On("GetClientSideID", "server-1").Return("client-1", nil).Times(3)
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		c.refuse = true

		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		s.queueBackoff = NewFixedBackoff(3, 0)

		replies := make(chan *domain.Reply, 1)
		replies <- &domain.Reply{RequestID: "server-1", Payload: []byte("done")}
		close(replies)

		s.writeReplies(replies)

		// the reply never reached the client, so deleting the entry would throw
		// away the only means of ever delivering it.
		registryMock.AssertNotCalled(t, "DeleteByServerSideID", "server-1")
	})

	t.Run("does not hold up the replies behind a client whose queue is full", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry
		registryMock.On("GetClientSideID", mock.Anything).Return("client-1", nil)
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		c.refuse = true

		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		s.replyBackoff = NewFixedBackoff(defaultReplyAttempts, defaultReplyWait)
		s.queueBackoff = NewFixedBackoff(defaultQueueAttempts, defaultQueueWait)

		replies := make(chan *domain.Reply, 3)
		for _, id := range []string{"server-1", "server-2", "server-3"} {
			replies <- &domain.Reply{RequestID: id, Payload: []byte("done")}
		}
		close(replies)

		start := time.Now()
		s.writeReplies(replies)

		assert.Less(t, time.Since(start), time.Second, "a full queue was waited out on the reply backoff")
	})

	t.Run("delivers once the client's queue drains", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry
		registryMock.On("GetClientSideID", "server-1").Return("client-1", nil)
		registryMock.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		c.refuse = true

		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		s.queueBackoff = NewFixedBackoff(5, 10*time.Millisecond)

		replies := make(chan *domain.Reply, 1)
		replies <- &domain.Reply{RequestID: "server-1", Payload: []byte("done")}
		close(replies)

		done := make(chan struct{})
		go func() {
			defer close(done)

			s.writeReplies(replies)
		}()

		// the client catches up before the attempts run out.
		time.Sleep(15 * time.Millisecond)
		c.lock.Lock()
		c.refuse = false
		c.lock.Unlock()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("the reply was never delivered after the queue drained")
		}

		written := c.written()
		assert.Len(t, written, 1)
		assert.Equal(t, "client-1", written[0].RequestID)
	})

	t.Run("stops waiting out the backoff once the websocket closes", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry
		registryMock.On("GetClientSideID", "server-1").Return("", errors.New("registry is unreachable"))

		c := newFakeConn()
		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		// long enough that finishing on time can only mean the close was noticed
		s.replyBackoff = NewFixedBackoff(10, time.Minute)

		replies := make(chan *domain.Reply, 1)
		replies <- &domain.Reply{RequestID: "server-1"}
		close(replies)

		done := make(chan struct{})
		go func() {
			defer close(done)

			s.writeReplies(replies)
		}()

		s.bus.Shutdown()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("a closing websocket had to wait out the full backoff")
		}
	})

	t.Run("leaves a reply it does not own to the session that does", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRegistry
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

	t.Run("frees the client's request id when the dispatch fails", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRegistry(8)

		var producerMock messagingMock.MockProduceConsumer
		producerMock.On("Produce", mock.Anything, protocol.BrokerSubject("runCode"), mock.Anything).
			Return(errors.New("the broker is unreachable")).Times(2)
		defer producerMock.AssertExpectations(t)

		translatorMock := echoTranslator()
		translatorMock.On("Translate", "error_on_processing_the_request", mock.AnythingOfType("[]func(*translator.Params)")).
			Return("error_on_processing_the_request").Times(2)

		request := domain.Request{ID: "1", Subject: "runCode", Payload: []byte(`{"id":"1"}`)}
		c := newFakeConn(request, request)

		newTestSession(c, registry, &producerMock, translatorMock, "runCode").Run(context.Background())

		written := c.written()
		require.Len(t, written, 2)

		for i, reply := range written {
			var response protocol.FailureResponse
			require.NoError(t, json.Unmarshal(reply.Payload, &response))

			assert.Equal(t, "error_on_processing_the_request", response.Error)
			assert.Empty(t, response.ValidationErrors, "reply %d rejected the id instead of reporting the produce failure", i)
		}

		_, err := registry.GetServerSideID("1")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a request that never reached the queue still holds the client's id")
	})

	t.Run("refuses a request once the reply path is gone", func(t *testing.T) {
		t.Parallel()

		var producerMock messagingMock.MockProduceConsumer
		defer producerMock.AssertExpectations(t)

		translatorMock := echoTranslator()
		translatorMock.On("Translate", "error_on_processing_the_request", mock.AnythingOfType("[]func(*translator.Params)")).
			Return("error_on_processing_the_request").Once()

		c := newFakeConn(domain.Request{ID: "1", Subject: "runCode", Payload: []byte(`{"id":"1"}`)})

		s := newTestSession(c, NewInMemoryRegistry(8), &producerMock, translatorMock, "runCode")
		s.bus.Shutdown()

		s.Run(context.Background())

		producerMock.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)

		written := c.written()
		require.Len(t, written, 1)

		var response protocol.FailureResponse
		require.NoError(t, json.Unmarshal(written[0].Payload, &response))
		assert.Equal(t, "error_on_processing_the_request", response.Error)
	})

	t.Run("a stalled client does not block the session writing replies", func(t *testing.T) {
		t.Parallel()

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		var registry MockRegistry
		registry.On("GetClientSideID", "server-1").Return("client-1", nil).Once()
		registry.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registry.AssertExpectations(t)

		conn := transport.NewConnection(
			websockettest.StalledClientConn(t),
			transport.Config{
				MaxMessageSize:   1024,
				WriteWait:        50 * time.Millisecond,
				PingPeriod:       time.Second,
				PongWait:         2 * time.Second,
				OutboundBuffer:   testOutboundBuffer,
				CloseGracePeriod: 50 * time.Millisecond,
			},
			logger,
		)
		defer conn.Shutdown()

		s := &Session{conn: conn, registry: &registry, logger: logger}

		replies := make(chan *domain.Reply, 1)
		replies <- &domain.Reply{RequestID: "server-1", Payload: []byte("payload")}
		close(replies)

		done := make(chan struct{})
		go func() {
			defer close(done)

			s.writeReplies(replies)
		}()

		select {
		case <-done:
			// PASS: handing the reply to the write pump did not wait on the
			// client, so the loop drained replies and returned.
		case <-time.After(time.Second):
			t.Fatal("writeReplies blocked indefinitely on a client that never reads")
		}
	})

	t.Run("survives a panic while writing replies", func(t *testing.T) {
		t.Parallel()

		c := &fakeConn{incoming: make(chan domain.Request)}

		s := newTestSession(c, panickingRegistry{}, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")

		done := make(chan struct{})
		go func() {
			defer close(done)

			s.Run(context.Background())
		}()

		require.Eventually(t, func() bool { return s.hub.Size() == 1 }, time.Second, 5*time.Millisecond)

		s.hub.Broadcast(&domain.Reply{RequestID: "server-1"})

		close(c.incoming)

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("the session never unwound after its reply goroutine panicked")
		}

		assert.Equal(t, 0, s.hub.Size(), "the session left its channel subscribed")
	})
}
