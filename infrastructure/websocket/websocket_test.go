package websocket

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/goleak"
)

// hijackableResponseWriter lets the gorilla Upgrader hand back a net.Conn that
// the test fully controls (e.g. one end of a net.Pipe).
type hijackableResponseWriter struct {
	*httptest.ResponseRecorder
	conn net.Conn
	bw   *bufio.ReadWriter
}

func (h *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, h.bw, nil
}

// staticRegistry hands every connection the same registry, so a test that dials
// a single client can keep its expectations on one mock. Production hands out a
// fresh registry per connection.
func staticRegistry(registry domain.RequestRegistry) func() domain.RequestRegistry {
	return func() domain.RequestRegistry { return registry }
}

// stalledClientConn upgrades a websocket over an in-memory pipe and returns the
// server side of it. net.Pipe is unbuffered, so once the client has drained the
// upgrade response and stopped reading, every server write blocks.
func stalledClientConn(t *testing.T) *websocket.Conn {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	upgradeDrained := make(chan struct{})
	go func() {
		defer close(upgradeDrained)

		buf := make([]byte, 4096)
		_, _ = clientConn.Read(buf)
	}()

	recorder := &hijackableResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		conn:             serverConn,
		bw: bufio.NewReadWriter(
			bufio.NewReader(serverConn),
			bufio.NewWriter(serverConn),
		),
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	conn, err := upgrader.Upgrade(recorder, request, nil)
	assert.NoError(t, err)

	<-upgradeDrained

	return conn
}

func TestWebsocket(t *testing.T) {
	t.Parallel()

	t.Run("connects to websocket successfully", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()
	})

	t.Run("refuses to be built with a configuration the protocol cannot work with", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		ws, err := NewWebsocket(
			staticRegistry(&requestRegistryMock),
			&produceConsumerMock,
			&publishSubscriberMock,
			&translatorMock,
			"replies",
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			// clients would be disconnected before they could answer a ping.
			WithPingPeriod(30*time.Second),
			WithPongWait(10*time.Second),
		)

		assert.Error(t, err)
		assert.Nil(t, ws)
		publishSubscriberMock.AssertNotCalled(t, "Subscribe", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("refuses new connections once it is closed", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		assert.NoError(t, ws.Close())

		// accepting here would produce work whose reply could never be carried
		// back, leaving the client waiting on an answer that cannot arrive.
		_, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.Error(t, err)
		if assert.NotNil(t, response) {
			defer response.Body.Close()
			assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
		}

		produceConsumerMock.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("gets error on http scheme request", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		request, err := http.NewRequest("GET", server.URL, nil)
		assert.NoError(t, err)

		client := http.Client{
			Timeout: 1 * time.Second,
		}

		response, err := client.Do(request)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	})

	t.Run("sends request and receives reply successfully", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler

			clientSideID = "client-side-id"
			serverSideID = "server-side-id"
		)

		message := domain.Request{
			ID:      clientSideID,
			Subject: "test",
			Payload: []byte(`{"message":"hello, world"}`),
		}

		requestRegistryMock.On("GetServerSideID", clientSideID).Return("", domain.ErrNotExists)
		requestRegistryMock.On("Add", clientSideID).Return(serverSideID, nil)
		requestRegistryMock.On("GetClientSideID", serverSideID).Return(clientSideID, nil)
		requestRegistryMock.On("DeleteByServerSideID", serverSideID).Return(nil).Maybe()
		defer requestRegistryMock.AssertExpectations(t)

		// Capture the reply handler registered during construction and wire it to Publish
		var replyHandler domain.MessageHandler
		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).
			Run(func(args mock.Arguments) {
				replyHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)
		publishSubscriberMock.On("Publish", mock.Anything, "websocket_replies", mock.Anything).
			Run(func(args mock.Arguments) {
				payload := args.Get(2).([]byte)
				replyHandler.Handle(context.Background(), payload)
			}).Return(nil)
		defer publishSubscriberMock.AssertExpectations(t)

		requestProcessed := make(chan struct{}, 1)
		produceConsumerMock.On("Consume", mock.Anything, "websocket_test", &messageHandlerMock).Return(nil)
		produceConsumerMock.On("Produce", mock.Anything, "websocket_test", mock.MatchedBy(func(payload []byte) bool {
			var data map[string]any
			assert.NoError(t, json.Unmarshal(payload, &data))
			assert.Equal(t, serverSideID, data["id"])
			assert.Equal(t, "hello, world", data["message"])
			return true
		})).Run(func(args mock.Arguments) {
			requestProcessed <- struct{}{}
		}).Return(nil)
		defer produceConsumerMock.AssertExpectations(t)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		ws.Consume(context.Background(), "test", &messageHandlerMock)

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		assert.NoError(t, client.WriteJSON(message))

		// wait for the server to process the request before publishing the reply
		<-requestProcessed

		// Payload must be a JSON-encoded domain.Reply for the reply subscription handler to unmarshal
		assert.NoError(t, ws.Reply(context.Background(), &domain.Reply{
			RequestID: serverSideID,
			Payload:   []byte("hello, this is websocket replying back"),
		}))

		var response domain.Reply
		assert.NoError(t, client.ReadJSON(&response))

		assert.Equal(t, message.ID, response.RequestID)
		assert.Equal(t, []byte("hello, this is websocket replying back"), response.Payload)
	})

	t.Run("gets error on invalid request", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		// Set up translator mock to return expected error messages
		translatorMock.On("Translate", "required_field", mock.AnythingOfType("[]func(*translator.Params)")).Return("required_field")
		translatorMock.On("Translate", "invalid_value", mock.AnythingOfType("[]func(*translator.Params)")).Return("invalid_value")
		defer translatorMock.AssertExpectations(t)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		testcases := []struct {
			name          string
			request       domain.Request
			expectedReply domain.Reply
		}{
			{
				// a missing id is reported as missing: the rule checking the
				// id's shape does not overrule the one checking it is there.
				name:          "empty_id",
				request:       domain.Request{ID: "", Subject: "test", Payload: []byte("hello, world")},
				expectedReply: domain.Reply{RequestID: "", Payload: []byte(`{"validationErrors":{"request_id":"required_field","subject":"invalid_value"}}`)},
			},
			{
				name:          "malformed_id",
				request:       domain.Request{ID: "not a valid id", Subject: "test", Payload: []byte("hello, world")},
				expectedReply: domain.Reply{RequestID: "not a valid id", Payload: []byte(`{"validationErrors":{"request_id":"invalid_value","subject":"invalid_value"}}`)},
			},
			{
				name:          "empty_subject",
				request:       domain.Request{ID: "123", Subject: "", Payload: []byte("hello, world")},
				expectedReply: domain.Reply{RequestID: "123", Payload: []byte(`{"validationErrors":{"subject":"required_field"}}`)},
			},
			{
				name:          "non_existing_subject",
				request:       domain.Request{ID: "123", Subject: "test", Payload: nil},
				expectedReply: domain.Reply{RequestID: "123", Payload: []byte(`{"validationErrors":{"subject":"invalid_value"}}`)},
			},
		}

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		for _, testcase := range testcases {
			t.Run(testcase.name, func(t *testing.T) {
				if testcase.request.ID != "" {
					requestRegistryMock.On("GetServerSideID", testcase.request.ID).Return("", domain.ErrNotExists)
					defer requestRegistryMock.AssertExpectations(t)
				}

				assert.NoError(t, client.WriteJSON(testcase.request))

				var response domain.Reply
				assert.NoError(t, client.ReadJSON(&response))

				assert.Equal(t, testcase.expectedReply.RequestID, response.RequestID)
				assert.JSONEq(t, string(testcase.expectedReply.Payload), string(response.Payload))
			})
		}
	})

	t.Run("reply returns error when closed", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		ws.Close()

		reply := domain.Reply{RequestID: "req-1", Payload: []byte("test")}

		err = ws.Reply(context.Background(), &reply)
		assert.EqualError(t, err, "connection is closed")
	})

	t.Run("reply returns error when request id is empty", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		reply := domain.Reply{Payload: []byte("test")}
		err = ws.Reply(context.Background(), &reply)
		assert.EqualError(t, err, "request id is required")
	})

	t.Run("consume propagates error from underlying produce/consumer", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		expectedErr := errors.New("consume failed")
		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)
		produceConsumerMock.On("Consume", mock.Anything, "websocket_test", &messageHandlerMock).Return(expectedErr)
		defer produceConsumerMock.AssertExpectations(t)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		err = ws.Consume(context.Background(), "test", &messageHandlerMock)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("consume registers subject for validation", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)
		produceConsumerMock.On("Consume", mock.Anything, "websocket_registered-subject", &messageHandlerMock).Return(nil)
		defer produceConsumerMock.AssertExpectations(t)

		translatorMock.On("Translate", "invalid_value", mock.AnythingOfType("[]func(*translator.Params)")).Return("invalid_value")
		defer translatorMock.AssertExpectations(t)

		requestRegistryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		defer requestRegistryMock.AssertExpectations(t)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		// consume on "registered-subject"
		err = ws.Consume(context.Background(), "registered-subject", &messageHandlerMock)
		assert.NoError(t, err)

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		// send request for a subject that was NOT registered
		assert.NoError(t, client.WriteJSON(domain.Request{
			ID:      "req-1",
			Subject: "unregistered-subject",
		}))

		var response domain.Reply
		assert.NoError(t, client.ReadJSON(&response))
		assert.JSONEq(t, `{"validationErrors":{"subject":"invalid_value"}}`, string(response.Payload))
	})

	t.Run("validation rejects duplicate request id", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)
		produceConsumerMock.On("Consume", mock.Anything, "websocket_test", &messageHandlerMock).Return(nil)
		produceConsumerMock.On("Produce", mock.Anything, "websocket_test", mock.Anything).Return(nil)
		defer produceConsumerMock.AssertExpectations(t)

		translatorMock.On("Translate", "request_already_exists", mock.AnythingOfType("[]func(*translator.Params)")).Return("request_already_exists")
		defer translatorMock.AssertExpectations(t)

		// First call: request does not exist yet
		requestRegistryMock.On("GetServerSideID", "req-dup").Return("", domain.ErrNotExists).Once()
		requestRegistryMock.On("Add", "req-dup").Return("server-dup", nil)
		requestRegistryMock.On("DeleteByServerSideID", "server-dup").Return(nil).Maybe()
		// Second call: request already exists
		requestRegistryMock.On("GetServerSideID", "req-dup").Return("server-dup", nil).Once()
		defer requestRegistryMock.AssertExpectations(t)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		ws.Consume(context.Background(), "test", &messageHandlerMock)

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		// first request succeeds
		assert.NoError(t, client.WriteJSON(domain.Request{ID: "req-dup", Subject: "test", Payload: []byte(`{}`)}))

		// allow time for the first request to be processed
		time.Sleep(100 * time.Millisecond)

		// second request with the same ID gets validation error
		assert.NoError(t, client.WriteJSON(domain.Request{ID: "req-dup", Subject: "test", Payload: []byte(`{}`)}))

		var response domain.Reply
		assert.NoError(t, client.ReadJSON(&response))
		assert.Equal(t, "req-dup", response.RequestID)
		assert.JSONEq(t, `{"validationErrors":{"request_id":"request_already_exists"}}`, string(response.Payload))
	})

	t.Run("close is idempotent", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)

		assert.NoError(t, ws.Close())
		assert.NoError(t, ws.Close())
	})

	t.Run("handles produce error on request forwarding", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		produceErr := errors.New("produce failed")

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)
		produceConsumerMock.On("Consume", mock.Anything, "websocket_test", &messageHandlerMock).Return(nil)
		produceConsumerMock.On("Produce", mock.Anything, "websocket_test", mock.Anything).Return(produceErr)
		defer produceConsumerMock.AssertExpectations(t)

		translatorMock.On("Translate", "error_on_processing_the_request", mock.AnythingOfType("[]func(*translator.Params)")).Return("error_on_processing_the_request")
		defer translatorMock.AssertExpectations(t)

		requestRegistryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		requestRegistryMock.On("Add", "req-1").Return("server-1", nil)
		requestRegistryMock.On("DeleteByServerSideID", "server-1").Return(nil).Maybe()
		defer requestRegistryMock.AssertExpectations(t)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		ws.Consume(context.Background(), "test", &messageHandlerMock)

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		assert.NoError(t, client.WriteJSON(domain.Request{ID: "req-1", Subject: "test", Payload: []byte(`{}`)}))

		var response domain.Reply
		assert.NoError(t, client.ReadJSON(&response))
		assert.Equal(t, "req-1", response.RequestID)
		assert.JSONEq(t, `{"error":"error_on_processing_the_request"}`, string(response.Payload))
	})

	t.Run("handles registry add error", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		addErr := errors.New("registry full")

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)
		produceConsumerMock.On("Consume", mock.Anything, "websocket_test", &messageHandlerMock).Return(nil)
		defer produceConsumerMock.AssertExpectations(t)

		translatorMock.On("Translate", "error_on_processing_the_request", mock.AnythingOfType("[]func(*translator.Params)")).Return("error_on_processing_the_request")
		defer translatorMock.AssertExpectations(t)

		requestRegistryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		requestRegistryMock.On("Add", "req-1").Return("", addErr)
		defer requestRegistryMock.AssertExpectations(t)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		ws.Consume(context.Background(), "test", &messageHandlerMock)

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		assert.NoError(t, client.WriteJSON(domain.Request{ID: "req-1", Subject: "test"}))

		var response domain.Reply
		assert.NoError(t, client.ReadJSON(&response))
		assert.Equal(t, "req-1", response.RequestID)
		assert.JSONEq(t, `{"error":"error_on_processing_the_request"}`, string(response.Payload))
	})

	t.Run("a stalled client does not block the session writing replies", func(t *testing.T) {
		t.Parallel()

		var registry MockRequestRegistry
		registry.On("GetClientSideID", "server-1").Return("client-1", nil).Once()
		registry.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registry.AssertExpectations(t)

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		config, err := newConfiguration(WithWriteWait(50 * time.Millisecond))
		assert.NoError(t, err)

		s := &session{
			conn:     newConnection(stalledClientConn(t), config, logger),
			registry: &registry,
			logger:   logger,
		}
		defer s.conn.shutdown()

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

	t.Run("a stalled client fills its own queue instead of blocking the sender", func(t *testing.T) {
		t.Parallel()

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		config, err := newConfiguration(
			WithWriteWait(50*time.Millisecond),
			WithOutboundBuffer(2),
		)
		assert.NoError(t, err)

		conn := newConnection(stalledClientConn(t), config, logger)
		defer conn.shutdown()

		// the client never reads, so the queue fills and then refuses more
		// rather than waiting for room: at most its buffer plus the one
		// message the write pump is stuck writing.
		const attempts = 10

		accepted := make(chan int, 1)
		go func() {
			count := 0
			for range attempts {
				if conn.send(&domain.Reply{RequestID: "server-1"}) {
					count++
				}
			}
			accepted <- count
		}()

		select {
		case count := <-accepted:
			assert.LessOrEqual(t, count, config.outboundBuffer+1, "send accepted more than the outbound queue can hold")
			assert.Less(t, count, attempts, "send should start refusing messages once the queue is full")
		case <-time.After(time.Second):
			t.Fatal("send blocked on a client that never reads")
		}
	})

	t.Run("a disconnected client's request ids do not follow it to the next connection", func(t *testing.T) {
		t.Parallel()

		var (
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)
		produceConsumerMock.On("Consume", mock.Anything, "websocket_test", &messageHandlerMock).Return(nil)

		produced := make(chan struct{}, 2)
		produceConsumerMock.On("Produce", mock.Anything, "websocket_test", mock.Anything).
			Run(func(args mock.Arguments) { produced <- struct{}{} }).
			Return(nil)

		// a real registry per connection, which is what production does.
		ws, err := NewWebsocket(
			func() domain.RequestRegistry { return NewInMemoryRequestRegistry(8) },
			&produceConsumerMock,
			&publishSubscriberMock,
			&translatorMock,
			"replies",
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)
		assert.NoError(t, err)
		defer ws.Close()

		ws.Consume(context.Background(), "test", &messageHandlerMock)

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		// the first client leaves without ever being answered, so its entry
		// would be the one to leak.
		first, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		assert.NoError(t, first.WriteJSON(domain.Request{ID: "req-1", Subject: "test", Payload: []byte(`{}`)}))
		<-produced
		first.Close()

		// the same id on a fresh connection must be accepted, not rejected as
		// one that already exists.
		second, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer second.Close()

		assert.NoError(t, second.WriteJSON(domain.Request{ID: "req-1", Subject: "test", Payload: []byte(`{}`)}))

		select {
		case <-produced:
		case <-time.After(2 * time.Second):
			t.Fatal("the second connection's request was never produced; its id was rejected as a leftover of the first")
		}

		produceConsumerMock.AssertExpectations(t)
		publishSubscriberMock.AssertExpectations(t)
	})

	t.Run("retries reply when GetClientSideID returns transient error", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		transientErr := errors.New("transient registry error")
		requestRegistryMock.On("GetClientSideID", "server-1").Return("", transientErr).Once()
		requestRegistryMock.On("GetClientSideID", "server-1").Return("client-1", nil).Once()

		deleted := make(chan struct{}, 1)
		requestRegistryMock.On("DeleteByServerSideID", "server-1").Run(func(args mock.Arguments) { deleted <- struct{}{} }).Return(nil).Once()
		defer requestRegistryMock.AssertExpectations(t)

		var replyHandler domain.MessageHandler
		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).
			Run(func(args mock.Arguments) {
				replyHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)
		publishSubscriberMock.On("Publish", mock.Anything, "websocket_replies", mock.Anything).
			Run(func(args mock.Arguments) {
				payload := args.Get(2).([]byte)
				replyHandler.Handle(context.Background(), payload)
			}).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		// give the server time to register this connection's responseChan
		// with the fanout before publishing the reply.
		time.Sleep(50 * time.Millisecond)

		assert.NoError(t, ws.Reply(context.Background(), &domain.Reply{
			RequestID: "server-1",
			Payload:   []byte("retry-success"),
		}))

		var response domain.Reply
		assert.NoError(t, client.ReadJSON(&response))
		assert.Equal(t, "client-1", response.RequestID)
		assert.Equal(t, []byte("retry-success"), response.Payload)

		// DeleteByServerSideID runs after the reply is written to the client, so
		// wait for it before the deferred AssertExpectations checks the mock.
		select {
		case <-deleted:
		case <-time.After(time.Second):
			t.Fatal("DeleteByServerSideID was not called after the reply was sent")
		}
	})

	t.Run("concurrent connect and disconnect does not corrupt the hub", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
		assert.NoError(t, err)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		const N = 30
		var wg sync.WaitGroup
		for range N {
			wg.Go(func() {
				client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
				if err != nil {
					t.Errorf("dial: %v", err)
					return
				}
				client.Close()
			})
		}
		wg.Wait()

		// the server unsubscribes each session from the hub asynchronously
		// after its client disconnects; poll until the hub drains or the
		// deadline expires.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if ws.hub.size() == 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}

		t.Fatalf("hub not cleaned up after all clients disconnected: %d subscribers remain", ws.hub.size())
	})
}

func TestNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	var (
		requestRegistryMock   MockRequestRegistry
		produceConsumerMock   messagingMock.MockProduceConsumer
		publishSubscriberMock messagingMock.MockPublishSubscriber
		translatorMock        translator.TranslatorMock
	)

	publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

	ws, err := NewWebsocket(staticRegistry(&requestRegistryMock), &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies", slog.New(slog.NewTextHandler(io.Discard, nil)))
	assert.NoError(t, err)

	server := httptest.NewServer(ws)

	u, err := url.Parse(server.URL)
	assert.NoError(t, err)
	u.Scheme = "ws"

	client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	assert.NoError(t, err)

	client.Close()
	server.Close()
	assert.NoError(t, ws.Close())
}
