package websocket

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

	t.Run("gets error on http scheme request", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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
				replyHandler.Handle(payload)
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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
				name:          "empty_id",
				request:       domain.Request{ID: "", Subject: "test", Payload: []byte("hello, world")},
				expectedReply: domain.Reply{RequestID: "", Payload: []byte(`{"validationErrors":{"request_id":"invalid_value","subject":"invalid_value"}}`)},
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

	t.Run("write deadline prevents writeResponses from blocking on slow client", func(t *testing.T) {
		t.Parallel()

		var registry MockRequestRegistry
		registry.On("GetClientSideID", "server-1").Return("client-1", nil).Once()
		registry.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer registry.AssertExpectations(t)

		w := &Websocket{
			requestRegistry:    &registry,
			websocketWriteWait: 50 * time.Millisecond,
		}

		// net.Pipe is synchronous and unbuffered: writes block until the
		// other end reads. We drain the HTTP 101 upgrade response on the
		// client side, then stop reading so subsequent server writes block.
		serverConn, clientConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		upgradeDrained := make(chan struct{})
		go func() {
			defer close(upgradeDrained)
			buf := make([]byte, 4096)
			_, _ = clientConn.Read(buf)
		}()

		rec := &hijackableResponseWriter{
			ResponseRecorder: httptest.NewRecorder(),
			conn:             serverConn,
			bw: bufio.NewReadWriter(
				bufio.NewReader(serverConn),
				bufio.NewWriter(serverConn),
			),
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Version", "13")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		wsConn, err := upgrader.Upgrade(rec, req, nil)
		assert.NoError(t, err)
		<-upgradeDrained

		responseChan := make(chan *domain.Reply, 1)
		responseChan <- &domain.Reply{RequestID: "server-1", Payload: []byte("payload")}
		close(responseChan)

		done := make(chan struct{})
		go func() {
			w.writeResponses(wsConn, responseChan)
			close(done)
		}()

		select {
		case <-done:
			// PASS: the write deadline expired, the WriteJSON failed, and the
			// loop drained responseChan and returned.
		case <-time.After(time.Second):
			t.Fatal("writeResponses blocked indefinitely; the write deadline did not fire")
		}
	})

	t.Run("registry is cleaned up when client disconnects before reply arrives", func(t *testing.T) {
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

		requestProcessed := make(chan struct{}, 1)
		produceConsumerMock.On("Produce", mock.Anything, "websocket_test", mock.Anything).
			Run(func(args mock.Arguments) { requestProcessed <- struct{}{} }).
			Return(nil)

		requestRegistryMock.On("GetServerSideID", "req-orphan").Return("", domain.ErrNotExists)
		requestRegistryMock.On("Add", "req-orphan").Return("server-orphan", nil)

		swept := make(chan struct{}, 1)
		requestRegistryMock.On("DeleteByServerSideID", "server-orphan").
			Run(func(args mock.Arguments) { swept <- struct{}{} }).
			Return(nil).Once()

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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

		assert.NoError(t, client.WriteJSON(domain.Request{ID: "req-orphan", Subject: "test", Payload: []byte(`{}`)}))

		// wait for the server to handle the request before disconnecting
		<-requestProcessed

		// disconnect WITHOUT waiting for a reply — the registry entry is now orphaned
		client.Close()

		select {
		case <-swept:
		case <-time.After(2 * time.Second):
			t.Fatal("DeleteByServerSideID was not called after client disconnect; sweep did not run")
		}

		requestRegistryMock.AssertExpectations(t)
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
		requestRegistryMock.On("DeleteByServerSideID", "server-1").Return(nil).Once()
		defer requestRegistryMock.AssertExpectations(t)

		var replyHandler domain.MessageHandler
		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).
			Run(func(args mock.Arguments) {
				replyHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)
		publishSubscriberMock.On("Publish", mock.Anything, "websocket_replies", mock.Anything).
			Run(func(args mock.Arguments) {
				payload := args.Get(2).([]byte)
				replyHandler.Handle(payload)
			}).Return(nil)

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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
	})

	t.Run("concurrent connect and disconnect does not corrupt replyChans", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			produceConsumerMock   messagingMock.MockProduceConsumer
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "websocket_replies", mock.Anything).Return(nil)

		ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
		assert.NoError(t, err)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		const N = 30
		var wg sync.WaitGroup
		for i := 0; i < N; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
				if err != nil {
					t.Errorf("dial: %v", err)
					return
				}
				client.Close()
			}()
		}
		wg.Wait()

		// the server cleans up replyChans asynchronously after each disconnect;
		// poll until the slice drains or the deadline expires.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			ws.lock.RLock()
			leftover := len(ws.replyChans)
			ws.lock.RUnlock()
			if leftover == 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}

		ws.lock.RLock()
		leftover := len(ws.replyChans)
		ws.lock.RUnlock()
		t.Fatalf("replyChans not cleaned up after all clients disconnected: %d entries remain", leftover)
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

	ws, err := NewWebsocket(&requestRegistryMock, &produceConsumerMock, &publishSubscriberMock, &translatorMock, "replies")
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
