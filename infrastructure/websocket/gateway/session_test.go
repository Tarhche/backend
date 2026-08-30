package gateway

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// fakeConn is an in-memory transport, so a session can be driven without a
// socket, an upgrade handshake or a port. It is also the shortest example of
// what a new transport has to implement.
type fakeConn struct {
	incoming chan domain.Request
	shut     chan struct{}

	lock     sync.Mutex
	outgoing []*domain.Reply
	closed   bool

	// refuse makes Send report a full queue, the way a client that is not
	// keeping up does.
	refuse bool
}

var _ Conn = &fakeConn{}

// newFakeConn plays a fixed script and then hangs up, the way a client that
// says its piece and leaves does.
func newFakeConn(requests ...domain.Request) *fakeConn {
	c := newIdleConn(requests...)
	close(c.incoming)

	return c
}

// newIdleConn plays its requests and then stays connected, waiting, until
// something closes it.
func newIdleConn(requests ...domain.Request) *fakeConn {
	incoming := make(chan domain.Request, len(requests)+1)
	for _, request := range requests {
		incoming <- request
	}

	return &fakeConn{incoming: incoming, shut: make(chan struct{})}
}

// Read drains the queued requests, then waits for the next one or for the
// connection to be closed.
func (c *fakeConn) Read(request *domain.Request) error {
	select {
	case queued, ok := <-c.incoming:
		if !ok {
			return io.EOF
		}

		*request = queued

		return nil

	case <-c.shut:
		return io.EOF
	}
}

func (c *fakeConn) Send(reply *domain.Reply) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.closed || c.refuse {
		return false
	}

	c.outgoing = append(c.outgoing, reply)

	return true
}

func (c *fakeConn) Close() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if !c.closed {
		c.closed = true

		// closing the transport is what releases a client parked in Read.
		close(c.shut)
	}

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

func (c *panickingConn) Read(*domain.Request) error {
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

func TestSession(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	newTestSession := func(c Conn, registry RequestRegistry, producer domain.Producer, tr *translator.TranslatorMock, consumedSubject string) *session {
		subjects := newSubjects()
		subjects.add(consumedSubject)

		return &session{
			conn: c,
			dispatcher: &dispatcher{
				validator:     newRequestValidator(registry, subjects, tr, defaultMaxInFlightRequests),
				registry:      registry,
				producer:      producer,
				subjectPrefix: defaultSubjectPrefix,
				logger:        logger,
			},
			registry:     registry,
			hub:          newHub(logger),
			bus:          newReplyBus(&messagingMock.MockPublishSubscriber{}, "replies", logger),
			replies:      make(chan *domain.Reply, defaultReplyBuffer),
			done:         make(chan struct{}),
			gone:         make(chan struct{}),
			replyBackoff: NewFixedBackoff(defaultReplyAttempts, 0),
			queueBackoff: NewFixedBackoff(defaultQueueAttempts, 0),
			translator:   tr,
			logger:       logger,
		}
	}

	// deliverAll hands the session the replies it would have received from the
	// hub and lets its writer run to completion.
	deliverAll := func(s *session, replies ...*domain.Reply) {
		for _, reply := range replies {
			s.replies <- reply
		}
		close(s.replies)

		s.writeReplies()
	}

	t.Run("dispatches a valid request onto the queue", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		registryMock.On("Len").Return(0)
		registryMock.On("Add", "req-1").Return("server-1", nil)
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
		registryMock.On("Len").Return(0)

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

	t.Run("refuses a request once the connection has more in flight than it may", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		registryMock.On("Len").Return(defaultMaxInFlightRequests)

		var producerMock messagingMock.MockProduceConsumer
		defer producerMock.AssertExpectations(t)

		c := newFakeConn(domain.Request{ID: "req-1", Subject: "runCode", Payload: []byte(`{}`)})

		newTestSession(c, &registryMock, &producerMock, echoTranslator(), "runCode").run(context.Background())

		written := c.written()
		require.Len(t, written, 1)
		assert.JSONEq(t, `{"validationErrors":{"request":"too_many_requests"}}`, string(written[0].Payload))

		producerMock.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
		registryMock.AssertNotCalled(t, "Add", mock.Anything)
	})

	t.Run("writes back the reply under the id the client chose", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetClientSideID", "server-1").Return("client-1", nil).Once()
		registryMock.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")

		deliverAll(s, &domain.Reply{RequestID: "server-1", Payload: []byte("done")})

		written := c.written()
		assert.Len(t, written, 1)
		assert.Equal(t, "client-1", written[0].RequestID)
		assert.Equal(t, []byte("done"), written[0].Payload)
	})

	t.Run("unwinds cleanly when the read loop panics", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry

		s := newTestSession(
			&panickingConn{fakeConn: newFakeConn()},
			&registryMock,
			&messagingMock.MockProduceConsumer{},
			echoTranslator(),
			"runCode",
		)

		func() {
			// net/http recovers a panic per connection, so the process lives on
			// and anything the session left behind lives on with it.
			defer func() { assert.NotNil(t, recover(), "expected the panic to reach the caller") }()

			s.run(context.Background())
		}()

		assert.Equal(t, 0, s.hub.size(), "the hub still holds this session, so its writeReplies goroutine can never be woken")
	})

	t.Run("stops retrying as soon as the client disconnects", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetClientSideID", "server-1").Return("", errors.New("registry is unreachable"))

		s := newTestSession(newFakeConn(), &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		// long enough that finishing quickly can only mean the disconnect was noticed
		s.replyBackoff = NewFixedBackoff(5, time.Minute)

		leave, joined := s.hub.join(s)
		require.True(t, joined)

		written := make(chan struct{})
		go func() {
			defer close(written)

			s.writeReplies()
		}()

		// a reply arrives that cannot be routed, so the session starts backing off
		s.hub.broadcast(&domain.Reply{RequestID: "server-1"})
		time.Sleep(20 * time.Millisecond)

		// the client goes away, which is what run does once readRequests returns
		start := time.Now()
		s.finish()
		leave()
		<-written

		assert.Less(t, time.Since(start), time.Second, "teardown waited out a backoff for a client that had already gone")
	})

	t.Run("gives up on a reply after the backoff runs out, and forgets the request", func(t *testing.T) {
		t.Parallel()

		unreachable := errors.New("registry is unreachable")

		var registryMock MockRequestRegistry
		// three attempts, then the reply is dropped: a fourth call would fail
		// the mock, which is the assertion.
		registryMock.On("GetClientSideID", "server-1").Return("", unreachable).Times(3)
		// nothing is going to answer this request now, so its id is freed for
		// the client to use again.
		registryMock.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		s.replyBackoff = NewFixedBackoff(3, 0)

		done := make(chan struct{})
		go func() {
			defer close(done)

			deliverAll(s, &domain.Reply{RequestID: "server-1"})
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("writeReplies never stopped retrying")
		}

		assert.Empty(t, c.written(), "an undeliverable reply should be dropped, not written to the wrong client")
	})

	t.Run("keeps the request addressable while the client's queue is full", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		// the entry survives every attempt: a lookup that came back empty would
		// have ended the retries after the first one.
		registryMock.On("GetClientSideID", "server-1").Return("client-1", nil).Times(3)
		registryMock.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		c.refuse = true

		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		s.queueBackoff = NewFixedBackoff(3, 0)

		deliverAll(s, &domain.Reply{RequestID: "server-1", Payload: []byte("done")})

		assert.Empty(t, c.written())
	})

	t.Run("does not hold up the replies behind a client whose queue is full", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetClientSideID", mock.Anything).Return("client-1", nil)
		registryMock.On("DeleteByServerSideID", mock.Anything).Return(nil)
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		c.refuse = true

		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		s.replyBackoff = NewFixedBackoff(defaultReplyAttempts, defaultReplyWait)
		s.queueBackoff = NewFixedBackoff(defaultQueueAttempts, defaultQueueWait)

		start := time.Now()
		deliverAll(s,
			&domain.Reply{RequestID: "server-1", Payload: []byte("done")},
			&domain.Reply{RequestID: "server-2", Payload: []byte("done")},
			&domain.Reply{RequestID: "server-3", Payload: []byte("done")},
		)

		assert.Less(t, time.Since(start), time.Second, "a full queue was waited out on the reply backoff")
	})

	t.Run("delivers once the client's queue drains", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetClientSideID", "server-1").Return("client-1", nil)
		registryMock.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		c.refuse = true

		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		s.queueBackoff = NewFixedBackoff(5, 10*time.Millisecond)

		done := make(chan struct{})
		go func() {
			defer close(done)

			deliverAll(s, &domain.Reply{RequestID: "server-1", Payload: []byte("done")})
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

	t.Run("stops waiting out the backoff once the gateway closes", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetClientSideID", "server-1").Return("", errors.New("registry is unreachable"))

		c := newFakeConn()
		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")
		// long enough that finishing on time can only mean the close was noticed
		s.replyBackoff = NewFixedBackoff(10, time.Minute)

		done := make(chan struct{})
		go func() {
			defer close(done)

			deliverAll(s, &domain.Reply{RequestID: "server-1"})
		}()

		s.bus.shutdown()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("a closing gateway had to wait out the full backoff")
		}
	})

	t.Run("leaves a reply it does not own to the session that does", func(t *testing.T) {
		t.Parallel()

		var registryMock MockRequestRegistry
		registryMock.On("GetClientSideID", "someone-elses-request").Return("", domain.ErrNotExists).Once()
		defer registryMock.AssertExpectations(t)

		c := newFakeConn()
		s := newTestSession(c, &registryMock, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")

		done := make(chan struct{})
		go func() {
			defer close(done)

			deliverAll(s, &domain.Reply{RequestID: "someone-elses-request"})
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

		registry := NewInMemoryRequestRegistry(8)

		var producerMock messagingMock.MockProduceConsumer
		producerMock.On("Produce", mock.Anything, "websocket_runCode", mock.Anything).
			Return(errors.New("the broker is unreachable")).Times(2)
		defer producerMock.AssertExpectations(t)

		translatorMock := echoTranslator()
		translatorMock.On("Translate", "error_on_processing_the_request", mock.AnythingOfType("[]func(*translator.Params)")).
			Return("error_on_processing_the_request").Times(2)

		request := domain.Request{ID: "1", Subject: "runCode", Payload: []byte(`{"id":"1"}`)}
		c := newFakeConn(request, request)

		newTestSession(c, registry, &producerMock, translatorMock, "runCode").run(context.Background())

		written := c.written()
		require.Len(t, written, 2)

		for i, reply := range written {
			var response failureResponse
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

		s := newTestSession(c, NewInMemoryRequestRegistry(8), &producerMock, translatorMock, "runCode")
		s.bus.shutdown()

		s.run(context.Background())

		producerMock.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)

		written := c.written()
		require.Len(t, written, 1)

		var response failureResponse
		require.NoError(t, json.Unmarshal(written[0].Payload, &response))
		assert.Equal(t, "error_on_processing_the_request", response.Error)
	})

	t.Run("survives a panic while writing replies", func(t *testing.T) {
		t.Parallel()

		c := newIdleConn()

		s := newTestSession(c, panickingRegistry{}, &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")

		done := make(chan struct{})
		go func() {
			defer close(done)

			s.run(context.Background())
		}()

		require.Eventually(t, func() bool { return s.hub.size() == 1 }, time.Second, 5*time.Millisecond)

		s.hub.broadcast(&domain.Reply{RequestID: "server-1"})

		c.Close()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("the session never unwound after its reply goroutine panicked")
		}

		assert.Equal(t, 0, s.hub.size(), "the session stayed in the hub")
	})

	t.Run("is stopped from outside by closing its connection", func(t *testing.T) {
		t.Parallel()

		c := newIdleConn()

		s := newTestSession(c, NewInMemoryRequestRegistry(8), &messagingMock.MockProduceConsumer{}, echoTranslator(), "runCode")

		go s.run(context.Background())

		require.Eventually(t, func() bool { return s.hub.size() == 1 }, time.Second, 5*time.Millisecond)

		stopped := make(chan struct{})
		go func() {
			defer close(stopped)

			s.stop()
		}()

		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("stop never released the session")
		}

		assert.Equal(t, 0, s.hub.size())
	})
}
