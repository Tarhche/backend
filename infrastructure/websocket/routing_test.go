package websocket

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// serverSideIDs records the id injected into each produced payload, keyed by the
// "who" field the test puts in the request, so a test can address a reply to a
// particular client's request without knowing the generated uuid.
type serverSideIDs struct {
	lock sync.Mutex
	ids  map[string]string
	seen chan string
}

func newServerSideIDs() *serverSideIDs {
	return &serverSideIDs{
		ids:  make(map[string]string),
		seen: make(chan string, 16),
	}
}

func (s *serverSideIDs) record(payload []byte) {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}

	who, _ := data["who"].(string)
	id, _ := data["id"].(string)

	s.lock.Lock()
	s.ids[who] = id
	s.lock.Unlock()

	s.seen <- who
}

func (s *serverSideIDs) get(who string) string {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.ids[who]
}

// twoClientServer stands up a websocket with a real per-connection registry and
// a publish/subscribe mock that loops published replies back in, the way the
// broker would.
func twoClientServer(t *testing.T) (*Websocket, *url.URL, *serverSideIDs) {
	t.Helper()

	var (
		produceConsumerMock   messagingMock.MockProduceConsumer
		publishSubscriberMock messagingMock.MockPublishSubscriber
		translatorMock        translator.TranslatorMock
		messageHandlerMock    messagingMock.MockMessageHandler
	)

	produced := newServerSideIDs()

	var replyHandler domain.MessageHandler
	publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).
		Run(func(args mock.Arguments) {
			replyHandler = args.Get(2).(domain.MessageHandler)
		}).Return(nil)
	publishSubscriberMock.On("Publish", mock.Anything, "websocket_replies", mock.Anything).
		Run(func(args mock.Arguments) {
			replyHandler.Handle(context.Background(), args.Get(2).([]byte))
		}).Return(nil)

	produceConsumerMock.On("Consume", mock.Anything, "websocket_test", &messageHandlerMock).Return(nil)
	produceConsumerMock.On("Produce", mock.Anything, "websocket_test", mock.Anything).
		Run(func(args mock.Arguments) { produced.record(args.Get(2).([]byte)) }).
		Return(nil)

	ws, err := NewWebsocket(
		&produceConsumerMock,
		&publishSubscriberMock,
		&translatorMock,
		"replies",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	assert.NoError(t, err)
	t.Cleanup(func() { ws.Close() })

	assert.NoError(t, ws.Consume(context.Background(), "test", &messageHandlerMock))

	server := httptest.NewServer(ws)
	t.Cleanup(server.Close)

	u, err := url.Parse(server.URL)
	assert.NoError(t, err)
	u.Scheme = "ws"

	return ws, u, produced
}

func TestReplyRouting(t *testing.T) {
	t.Parallel()

	t.Run("a reply reaches only the client that asked for it", func(t *testing.T) {
		t.Parallel()

		ws, u, produced := twoClientServer(t)

		clientA, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer clientA.Close()

		clientB, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer clientB.Close()

		assert.NoError(t, clientA.WriteJSON(domain.Request{ID: "a-1", Subject: "test", Payload: []byte(`{"who":"A"}`)}))
		assert.NoError(t, clientB.WriteJSON(domain.Request{ID: "b-1", Subject: "test", Payload: []byte(`{"who":"B"}`)}))

		for range 2 {
			select {
			case <-produced.seen:
			case <-time.After(2 * time.Second):
				t.Fatal("requests were not produced")
			}
		}

		// answer B's request only.
		assert.NoError(t, ws.Reply(context.Background(), &domain.Reply{
			RequestID: produced.get("B"),
			Payload:   []byte(`{"secret":"for-client-B"}`),
		}))

		var reply domain.Reply
		assert.NoError(t, clientB.ReadJSON(&reply))
		assert.Equal(t, "b-1", reply.RequestID, "B should be answered under the id it chose")
		assert.JSONEq(t, `{"secret":"for-client-B"}`, string(reply.Payload))

		// A must not see it. Its registry never held B's request, so the reply
		// is not addressable there.
		assert.NoError(t, clientA.SetReadDeadline(time.Now().Add(400*time.Millisecond)))

		var leaked domain.Reply
		err = clientA.ReadJSON(&leaked)
		assert.Error(t, err, "client A received a reply meant for client B: %+v", leaked)
	})

	t.Run("every connection is given its own registry", func(t *testing.T) {
		t.Parallel()

		// this is the invariant the other two rest on, and the only one that
		// can be asserted without racing two sessions against each other: a
		// session cannot resolve another's reply if it never shares a registry.
		var (
			created               atomic.Int32
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(
			&produceConsumerMock,
			&publishSubscriberMock,
			&translatorMock,
			"replies",
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			WithRequestRegistry(func() RequestRegistry {
				created.Add(1)

				return NewInMemoryRequestRegistry(8)
			}),
		)
		assert.NoError(t, err)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		const connections = 4
		for range connections {
			client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			assert.NoError(t, err)
			defer client.Close()
		}

		assert.Eventually(t, func() bool {
			return created.Load() == connections
		}, 2*time.Second, 10*time.Millisecond, "expected one registry per connection, got %d for %d connections", created.Load(), connections)
	})

	t.Run("two clients may choose the same request id at the same time", func(t *testing.T) {
		t.Parallel()

		ws, u, produced := twoClientServer(t)

		clientA, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer clientA.Close()

		clientB, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer clientB.Close()

		// the same client-side id, both in flight at once.
		assert.NoError(t, clientA.WriteJSON(domain.Request{ID: "1", Subject: "test", Payload: []byte(`{"who":"A"}`)}))
		assert.NoError(t, clientB.WriteJSON(domain.Request{ID: "1", Subject: "test", Payload: []byte(`{"who":"B"}`)}))

		for range 2 {
			select {
			case <-produced.seen:
			case <-time.After(2 * time.Second):
				t.Fatal("both requests should have been produced; one was rejected as a duplicate id")
			}
		}

		serverA, serverB := produced.get("A"), produced.get("B")
		assert.NotEmpty(t, serverA)
		assert.NotEmpty(t, serverB)
		assert.NotEqual(t, serverA, serverB, "server-side ids must be unique across clients")

		// each client is answered its own reply, under the id it chose.
		assert.NoError(t, ws.Reply(context.Background(), &domain.Reply{RequestID: serverA, Payload: []byte(`"A"`)}))

		var reply domain.Reply
		assert.NoError(t, clientA.ReadJSON(&reply))
		assert.Equal(t, "1", reply.RequestID)
		assert.JSONEq(t, `"A"`, string(reply.Payload))
	})

	t.Run("server side ids are unique across connections", func(t *testing.T) {
		t.Parallel()

		_, u, produced := twoClientServer(t)

		const (
			clients           = 5
			requestsPerClient = 10
		)

		var wg sync.WaitGroup
		for c := range clients {
			wg.Go(func() {
				client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
				if err != nil {
					t.Errorf("dial: %v", err)
					return
				}
				defer client.Close()

				for r := range requestsPerClient {
					// every client walks the same id sequence on purpose.
					payload := []byte(`{"who":"` + string(rune('A'+c)) + string(rune('0'+r)) + `"}`)
					if err := client.WriteJSON(domain.Request{
						ID:      "req-" + string(rune('0'+r)),
						Subject: "test",
						Payload: payload,
					}); err != nil {
						t.Errorf("write: %v", err)
						return
					}
				}
			})
		}
		wg.Wait()

		for range clients * requestsPerClient {
			select {
			case <-produced.seen:
			case <-time.After(3 * time.Second):
				t.Fatal("not every request was produced; ids collided across clients")
			}
		}

		produced.lock.Lock()
		defer produced.lock.Unlock()

		unique := make(map[string]struct{}, len(produced.ids))
		for who, id := range produced.ids {
			assert.NotEmpty(t, id, "request %q got no server-side id", who)
			unique[id] = struct{}{}
		}

		assert.Equal(t, clients*requestsPerClient, len(produced.ids), "every request should have been recorded")
		assert.Equal(t, len(produced.ids), len(unique), "server-side ids must be unique across every client")
	})
}
