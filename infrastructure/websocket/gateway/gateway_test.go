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

const testSubject = "runCode"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// produced records the server-side id injected into each produced payload,
// keyed by the "who" field the test puts in the request, so a test can address
// a reply to a particular client's request without knowing the generated uuid.
type produced struct {
	lock sync.Mutex
	ids  map[string]string
	seen chan string
}

func newProduced() *produced {
	return &produced{
		ids:  make(map[string]string),
		seen: make(chan string, 64),
	}
}

func (p *produced) record(payload []byte) {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}

	who, _ := data["who"].(string)
	id, _ := data["id"].(string)

	p.lock.Lock()
	p.ids[who] = id
	p.lock.Unlock()

	p.seen <- who
}

func (p *produced) get(who string) string {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.ids[who]
}

func (p *produced) await(t *testing.T, count int) {
	t.Helper()

	for range count {
		select {
		case <-p.seen:
		case <-time.After(5 * time.Second):
			t.Fatal("requests were not produced")
		}
	}
}

// testGateway stands up a gateway whose publish/subscribe loops replies back
// in, the way the broker would, and whose producer records what it was given.
func testGateway(t *testing.T, options ...Option) (*Gateway, *produced) {
	t.Helper()

	var (
		produceConsumerMock   messagingMock.MockProduceConsumer
		publishSubscriberMock messagingMock.MockPublishSubscriber
		messageHandlerMock    messagingMock.MockMessageHandler
	)

	requests := newProduced()

	var replyHandler domain.MessageHandler
	publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).
		Run(func(args mock.Arguments) {
			replyHandler = args.Get(2).(domain.MessageHandler)
		}).Return(nil)
	publishSubscriberMock.On("Publish", mock.Anything, "websocket_replies", mock.Anything).
		Run(func(args mock.Arguments) {
			_ = replyHandler.Handle(context.Background(), args.Get(2).([]byte))
		}).Return(nil)

	produceConsumerMock.On("Consume", mock.Anything, "websocket_"+testSubject, &messageHandlerMock).Return(nil)
	produceConsumerMock.On("Produce", mock.Anything, "websocket_"+testSubject, mock.Anything).
		Run(func(args mock.Arguments) { requests.record(args.Get(2).([]byte)) }).
		Return(nil)

	g, err := New(&produceConsumerMock, &publishSubscriberMock, echoTranslator(), "replies", discardLogger(), options...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = g.Close() })

	require.NoError(t, g.Consume(context.Background(), testSubject, &messageHandlerMock))

	return g, requests
}

// serve drives a client on the gateway and hands back the connection and a
// channel closed once that client's session has fully unwound.
func serve(g *Gateway, requests ...domain.Request) (*fakeConn, chan struct{}) {
	c := newIdleConn(requests...)

	served := make(chan struct{})
	go func() {
		defer close(served)

		g.Serve(context.Background(), c)
	}()

	return c, served
}

func TestGateway(t *testing.T) {
	t.Parallel()

	t.Run("carries a reply back to the client that asked for it", func(t *testing.T) {
		t.Parallel()

		g, requests := testGateway(t)

		c, served := serve(g, domain.Request{ID: "1", Subject: testSubject, Payload: []byte(`{"who":"a"}`)})
		requests.await(t, 1)

		require.NoError(t, g.Reply(context.Background(), &domain.Reply{
			RequestID: requests.get("a"),
			Payload:   []byte(`{"ok":true}`),
		}))

		require.Eventually(t, func() bool { return len(c.written()) == 1 }, 2*time.Second, 5*time.Millisecond)

		written := c.written()
		assert.Equal(t, "1", written[0].RequestID, "the client is answered under the id it chose")
		assert.JSONEq(t, `{"ok":true}`, string(written[0].Payload))

		_ = c.Close()
		<-served
	})

	t.Run("refuses to be built with a configuration it cannot work with", func(t *testing.T) {
		t.Parallel()

		g, err := New(
			&messagingMock.MockProduceConsumer{},
			&messagingMock.MockPublishSubscriber{},
			&translator.TranslatorMock{},
			"replies",
			discardLogger(),
			WithReplyBuffer(0),
		)

		assert.Error(t, err)
		assert.Nil(t, g)
	})

	t.Run("consume propagates the error from the underlying consumer", func(t *testing.T) {
		t.Parallel()

		var (
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		expected := errors.New("the broker is unreachable")

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)
		produceConsumerMock.On("Consume", mock.Anything, "websocket_"+testSubject, &messageHandlerMock).Return(expected)

		g, err := New(&produceConsumerMock, &publishSubscriberMock, echoTranslator(), "replies", discardLogger())
		require.NoError(t, err)
		defer g.Close()

		assert.ErrorIs(t, g.Consume(context.Background(), testSubject, &messageHandlerMock), expected)
		assert.False(t, g.subjects.has(testSubject), "a subject that could not be consumed must not become sendable")
	})

	t.Run("refuses a reply that names no request", func(t *testing.T) {
		t.Parallel()

		g, _ := testGateway(t)

		assert.ErrorIs(t, g.Reply(context.Background(), &domain.Reply{}), ErrRequestIDRequired)
	})

	t.Run("refuses to accept or reply once it is closed", func(t *testing.T) {
		t.Parallel()

		g, _ := testGateway(t)

		assert.NoError(t, g.Accept())
		assert.NoError(t, g.Close())

		assert.ErrorIs(t, g.Accept(), ErrClosed)
		assert.ErrorIs(t, g.Reply(context.Background(), &domain.Reply{RequestID: "1"}), ErrClosed)
	})

	t.Run("closing is idempotent", func(t *testing.T) {
		t.Parallel()

		g, _ := testGateway(t)

		assert.NoError(t, g.Close())
		assert.NoError(t, g.Close())
	})

	t.Run("closing disconnects the clients it is serving", func(t *testing.T) {
		t.Parallel()

		g, _ := testGateway(t)

		const clients = 5

		conns := make([]*fakeConn, 0, clients)
		served := make([]chan struct{}, 0, clients)

		for range clients {
			// a client that sits there, sending nothing, the way an idle
			// browser tab does.
			c := newIdleConn()

			done := make(chan struct{})
			go func() {
				defer close(done)

				g.Serve(context.Background(), c)
			}()

			conns = append(conns, c)
			served = append(served, done)
		}

		require.Eventually(t, func() bool { return g.hub.size() == clients }, 2*time.Second, 5*time.Millisecond)

		require.NoError(t, g.Close())

		// Close returns only once the sessions are gone, so nothing is left
		// holding a client the gateway can no longer answer.
		assert.Equal(t, 0, g.hub.size())

		for i, done := range served {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("session %d was still running after the gateway closed", i)
			}
		}

		for i, c := range conns {
			c.lock.Lock()
			closed := c.closed
			c.lock.Unlock()

			assert.True(t, closed, "connection %d was left open", i)
		}
	})

	t.Run("a client accepted as the gateway closes is not left stranded", func(t *testing.T) {
		t.Parallel()

		g, _ := testGateway(t)

		require.NoError(t, g.Close())

		// Accept would have refused this client; serving one anyway is the
		// race between the two, and it must still end.
		c, served := serve(g)

		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Fatal("a session started after the shutdown never ended")
		}

		c.lock.Lock()
		defer c.lock.Unlock()
		assert.True(t, c.closed)
	})
}
