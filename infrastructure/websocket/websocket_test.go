package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/khanzadimahdi/testproject/domain"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

const testSubject = "runCode"

// fakeSessions stands in for the gateway, so the handler can be tested for what
// it is: an upgrade, a refusal, and a hand-over.
type fakeSessions struct {
	accept error

	lock   sync.Mutex
	served []gateway.Conn

	// onServe runs on the handler's goroutine, in place of a real session.
	onServe func(gateway.Conn)
}

var _ gateway.Sessions = &fakeSessions{}

func (f *fakeSessions) Accept() error {
	return f.accept
}

func (f *fakeSessions) Serve(_ context.Context, conn gateway.Conn) {
	f.lock.Lock()
	f.served = append(f.served, conn)
	f.lock.Unlock()

	if f.onServe != nil {
		f.onServe(conn)

		return
	}

	conn.Close()
}

func (f *fakeSessions) count() int {
	f.lock.Lock()
	defer f.lock.Unlock()

	return len(f.served)
}

// wsURL turns a test server's address into one a websocket client can dial.
func wsURL(t *testing.T, server *httptest.Server) *url.URL {
	t.Helper()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	u.Scheme = "ws"

	return u
}

func TestHandler(t *testing.T) {
	t.Parallel()

	t.Run("hands an upgraded client to the gateway", func(t *testing.T) {
		t.Parallel()

		sessions := &fakeSessions{
			onServe: func(conn gateway.Conn) {
				conn.Send(&domain.Reply{RequestID: "hello"})
				conn.Close()
			},
		}

		handler, err := NewHandler(sessions, discardLogger())
		require.NoError(t, err)

		server := httptest.NewServer(handler)
		defer server.Close()

		client, _, err := websocket.DefaultDialer.Dial(wsURL(t, server).String(), nil)
		require.NoError(t, err)
		defer client.Close()

		require.NoError(t, client.SetReadDeadline(time.Now().Add(2*time.Second)))

		var reply domain.Reply
		require.NoError(t, client.ReadJSON(&reply))

		assert.Equal(t, "hello", reply.RequestID)
		assert.Equal(t, 1, sessions.count())
	})

	t.Run("refuses a client the gateway will not accept", func(t *testing.T) {
		t.Parallel()

		sessions := &fakeSessions{accept: gateway.ErrClosed}

		handler, err := NewHandler(sessions, discardLogger())
		require.NoError(t, err)

		server := httptest.NewServer(handler)
		defer server.Close()

		_, response, err := websocket.DefaultDialer.Dial(wsURL(t, server).String(), nil)

		require.Error(t, err)
		require.NotNil(t, response)
		assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
		assert.Equal(t, 0, sessions.count(), "a refused client should never reach the gateway")
	})

	t.Run("rejects a request that is not an upgrade", func(t *testing.T) {
		t.Parallel()

		sessions := &fakeSessions{}

		handler, err := NewHandler(sessions, discardLogger())
		require.NoError(t, err)

		server := httptest.NewServer(handler)
		defer server.Close()

		response, err := http.Get(server.URL)
		require.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, 0, sessions.count())
	})

	t.Run("honours the origin check", func(t *testing.T) {
		t.Parallel()

		sessions := &fakeSessions{}

		handler, err := NewHandler(sessions, discardLogger(), WithOriginChecker(func(*http.Request) bool { return false }))
		require.NoError(t, err)

		server := httptest.NewServer(handler)
		defer server.Close()

		_, response, err := websocket.DefaultDialer.Dial(wsURL(t, server).String(), nil)

		require.Error(t, err)
		require.NotNil(t, response)
		assert.Equal(t, http.StatusForbidden, response.StatusCode)
		assert.Equal(t, 0, sessions.count())
	})

	t.Run("refuses to be built with a configuration the protocol cannot work with", func(t *testing.T) {
		t.Parallel()

		handler, err := NewHandler(&fakeSessions{}, discardLogger(), WithPingPeriod(defaultPongWait))

		assert.Error(t, err)
		assert.Nil(t, handler)
	})
}

// testServer wires a real gateway to a real websocket handler over real
// sockets: the closest a test gets to what runs in production.
func testServer(t *testing.T) (*gateway.Gateway, *httptest.Server, *url.URL, chan []byte) {
	t.Helper()

	var (
		produceConsumerMock   messagingMock.MockProduceConsumer
		publishSubscriberMock messagingMock.MockPublishSubscriber
		messageHandlerMock    messagingMock.MockMessageHandler
		translatorMock        translator.TranslatorMock
	)

	translatorMock.On("Translate", mock.Anything, mock.Anything).Return("translated").Maybe()

	produced := make(chan []byte, 128)

	var replyHandler domain.MessageHandler
	publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).
		Run(func(args mock.Arguments) { replyHandler = args.Get(2).(domain.MessageHandler) }).
		Return(nil)
	publishSubscriberMock.On("Publish", mock.Anything, "websocket_replies", mock.Anything).
		Run(func(args mock.Arguments) { _ = replyHandler.Handle(context.Background(), args.Get(2).([]byte)) }).
		Return(nil)

	produceConsumerMock.On("Consume", mock.Anything, "websocket_"+testSubject, &messageHandlerMock).Return(nil)
	produceConsumerMock.On("Produce", mock.Anything, "websocket_"+testSubject, mock.Anything).
		Run(func(args mock.Arguments) { produced <- args.Get(2).([]byte) }).
		Return(nil)

	g, err := gateway.New(&produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", discardLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = g.Close() })

	require.NoError(t, g.Consume(context.Background(), testSubject, &messageHandlerMock))

	handler, err := NewHandler(g, discardLogger())
	require.NoError(t, err)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return g, server, wsURL(t, server), produced
}

// serverSideID reads the id the dispatcher injected into a produced payload.
func serverSideID(t *testing.T, produced chan []byte) string {
	t.Helper()

	select {
	case payload := <-produced:
		var data map[string]any
		require.NoError(t, json.Unmarshal(payload, &data))

		id, _ := data["id"].(string)
		require.NotEmpty(t, id)

		return id

	case <-time.After(5 * time.Second):
		t.Fatal("the request was never produced")

		return ""
	}
}

func TestWebsocketEndToEnd(t *testing.T) {
	t.Parallel()

	t.Run("a request is produced and its reply comes back under the client's id", func(t *testing.T) {
		t.Parallel()

		g, _, u, produced := testServer(t)

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		defer client.Close()

		require.NoError(t, client.WriteJSON(domain.Request{ID: "req-1", Subject: testSubject, Payload: []byte(`{"code":"print(1)"}`)}))

		id := serverSideID(t, produced)

		require.NoError(t, g.Reply(context.Background(), &domain.Reply{RequestID: id, Payload: []byte(`{"output":"1"}`)}))

		require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))

		var reply domain.Reply
		require.NoError(t, client.ReadJSON(&reply))

		assert.Equal(t, "req-1", reply.RequestID)
		assert.JSONEq(t, `{"output":"1"}`, string(reply.Payload))
	})

	t.Run("an invalid request is answered rather than dispatched", func(t *testing.T) {
		t.Parallel()

		_, _, u, produced := testServer(t)

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		defer client.Close()

		require.NoError(t, client.WriteJSON(domain.Request{ID: "req-1", Subject: "nobody-listens-here"}))

		require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))

		var reply domain.Reply
		require.NoError(t, client.ReadJSON(&reply))

		assert.Equal(t, "req-1", reply.RequestID)
		assert.Contains(t, string(reply.Payload), "validationErrors")

		select {
		case <-produced:
			t.Fatal("a rejected request reached the queue")
		default:
		}
	})

	t.Run("closing the gateway disconnects the clients it is serving", func(t *testing.T) {
		t.Parallel()

		g, _, u, _ := testServer(t)

		const clients = 5

		conns := make([]*websocket.Conn, 0, clients)
		for range clients {
			client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			require.NoError(t, err)

			conns = append(conns, client)
		}

		// give the sessions a moment to reach the hub
		time.Sleep(100 * time.Millisecond)

		require.NoError(t, g.Close())

		for i, client := range conns {
			require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))

			_, _, err := client.ReadMessage()
			assert.Error(t, err, "client %d was left holding a connection the gateway can no longer answer", i)

			client.Close()
		}

		// and a client arriving afterwards is turned away rather than upgraded
		_, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.Error(t, err)
		require.NotNil(t, response)
		assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	})

	t.Run("a client that never reads does not hold up the reply path", func(t *testing.T) {
		t.Parallel()

		g, _, u, produced := testServer(t)

		stalled, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		defer stalled.Close()

		const requests = 30

		for i := range requests {
			require.NoError(t, stalled.WriteJSON(domain.Request{
				ID:      fmt.Sprintf("req-%d", i),
				Subject: testSubject,
				Payload: []byte(`{}`),
			}))
		}

		ids := make([]string, 0, requests)
		for range requests {
			ids = append(ids, serverSideID(t, produced))
		}

		// the client is not reading any of these back
		start := time.Now()
		for _, id := range ids {
			require.NoError(t, g.Reply(context.Background(), &domain.Reply{RequestID: id, Payload: []byte(`{}`)}))
		}

		assert.Less(t, time.Since(start), 5*time.Second, "a client that was not reading blocked the reply path")
	})

	t.Run("clients coming and going leave nothing behind", func(t *testing.T) {
		t.Parallel()

		g, _, u, _ := testServer(t)

		var group sync.WaitGroup
		var failures atomic.Int64

		for range 25 {

			group.Go(func() {

				for range 2 {
					client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
					if err != nil {
						failures.Add(1)

						return
					}

					_ = client.WriteJSON(domain.Request{ID: "1", Subject: testSubject, Payload: []byte(`{}`)})
					client.Close()
				}
			})
		}

		group.Wait()

		assert.Zero(t, failures.Load())

		require.Eventually(t, func() bool { return g.ConnectedClients() == 0 }, 5*time.Second, 10*time.Millisecond,
			"sessions were left in the hub after their clients disconnected")
	})
}

func TestNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	g, server, u, _ := testServer(t)

	for range 10 {
		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)

		_ = client.WriteJSON(domain.Request{ID: "1", Subject: testSubject, Payload: []byte(`{}`)})
		client.Close()
	}

	require.NoError(t, g.Close())

	server.Close()
}
