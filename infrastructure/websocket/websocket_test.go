package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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
}
