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
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		url, err := url.Parse(server.URL)
		assert.NoError(t, err)
		url.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(url.String(), nil)
		assert.NoError(t, err)
		defer client.Close()
	})

	t.Run("gets error on http scheme request", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
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
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler

			clientSideID = "client-side-id"
			serverSideID = "server-side-id"
			expectedCtx  = mock.Anything
		)

		message := domain.Request{
			ID:      clientSideID,
			Subject: "test",
			Payload: []byte("hello, world"),
		}

		requestRegistryMock.On("GetServerSideID", clientSideID).Return("", domain.ErrNotExists)
		requestRegistryMock.On("Add", clientSideID).Return(serverSideID, nil)
		requestRegistryMock.On("GetClientSideID", serverSideID).Return(clientSideID, nil)
		requestRegistryMock.On("DeleteByServerSideID", serverSideID).Return(nil).Maybe()
		defer requestRegistryMock.AssertExpectations(t)

		requestProcessed := make(chan struct{}, 1)
		publishSubscriberMock.On("Subscribe", expectedCtx, "test", &messageHandlerMock).Return(nil)
		publishSubscriberMock.On("Publish", expectedCtx, "test", mock.MatchedBy(func(payload []byte) bool {
			var request domain.Request
			assert.NoError(t, json.Unmarshal(payload, &request))
			assert.NotEqual(t, message.ID, request.ID)
			assert.Equal(t, message.Subject, request.Subject)
			assert.Equal(t, message.Payload, request.Payload)
			return true
		})).Run(func(args mock.Arguments) {
			requestProcessed <- struct{}{}
		}).Return(nil)
		defer publishSubscriberMock.AssertExpectations(t)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		ws.Subscribe(context.Background(), "test", &messageHandlerMock)

		server := httptest.NewServer(ws)
		defer server.Close()

		url, err := url.Parse(server.URL)
		assert.NoError(t, err)
		url.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(url.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		assert.NoError(t, client.WriteJSON(message))

		// wait for the server to process the request before publishing the reply
		<-requestProcessed

		reply := domain.Reply{
			RequestID: serverSideID,
			Payload:   []byte("hello, this is websocket replying back"),
		}
		replyJson, err := json.Marshal(&reply)
		assert.NoError(t, err)

		assert.NoError(t, ws.Publish(context.Background(), "test", replyJson))

		var response domain.Reply
		assert.NoError(t, client.ReadJSON(&response))

		assert.Equal(t, message.ID, response.RequestID)
		assert.Equal(t, []byte("hello, this is websocket replying back"), response.Payload)
	})

	t.Run("gets error on invalid request", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		// Set up translator mock to return expected error messages
		translatorMock.On("Translate", "required_field", mock.AnythingOfType("[]func(*translator.Params)")).Return("required_field")
		translatorMock.On("Translate", "invalid_value", mock.AnythingOfType("[]func(*translator.Params)")).Return("invalid_value")
		defer translatorMock.AssertExpectations(t)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		server := httptest.NewServer(ws)
		defer server.Close()

		url, err := url.Parse(server.URL)
		assert.NoError(t, err)
		url.Scheme = "ws"

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

		client, _, err := websocket.DefaultDialer.Dial(url.String(), nil)
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

	t.Run("publish returns error when closed", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		ws.Close()

		reply := domain.Reply{RequestID: "req-1", Payload: []byte("test")}
		payload, err := json.Marshal(&reply)
		assert.NoError(t, err)

		err = ws.Publish(context.Background(), "test", payload)
		assert.EqualError(t, err, "connection is closed")
	})

	t.Run("publish returns error on invalid json", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		err := ws.Publish(context.Background(), "test", []byte("invalid json"))
		assert.Error(t, err)
	})

	t.Run("publish returns error when request id is empty", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		reply := domain.Reply{RequestID: "", Payload: []byte("test")}
		payload, err := json.Marshal(&reply)
		assert.NoError(t, err)

		err = ws.Publish(context.Background(), "test", payload)
		assert.EqualError(t, err, "request id is required")
	})

	t.Run("subscribe propagates error from underlying pub/sub", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		expectedErr := errors.New("subscribe failed")
		publishSubscriberMock.On("Subscribe", mock.Anything, "test", &messageHandlerMock).Return(expectedErr)
		defer publishSubscriberMock.AssertExpectations(t)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		err := ws.Subscribe(context.Background(), "test", &messageHandlerMock)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("subscribe registers subject for validation", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "registered-subject", &messageHandlerMock).Return(nil)
		defer publishSubscriberMock.AssertExpectations(t)

		translatorMock.On("Translate", "invalid_value", mock.AnythingOfType("[]func(*translator.Params)")).Return("invalid_value")
		defer translatorMock.AssertExpectations(t)

		requestRegistryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		defer requestRegistryMock.AssertExpectations(t)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		// subscribe to "registered-subject"
		err := ws.Subscribe(context.Background(), "registered-subject", &messageHandlerMock)
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
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		publishSubscriberMock.On("Subscribe", mock.Anything, "test", &messageHandlerMock).Return(nil)
		publishSubscriberMock.On("Publish", mock.Anything, "test", mock.Anything).Return(nil)
		defer publishSubscriberMock.AssertExpectations(t)

		translatorMock.On("Translate", "request_already_exists", mock.AnythingOfType("[]func(*translator.Params)")).Return("request_already_exists")
		defer translatorMock.AssertExpectations(t)

		// First call: request does not exist yet
		requestRegistryMock.On("GetServerSideID", "req-dup").Return("", domain.ErrNotExists).Once()
		requestRegistryMock.On("Add", "req-dup").Return("server-dup", nil)
		requestRegistryMock.On("DeleteByServerSideID", "server-dup").Return(nil).Maybe()
		// Second call: request already exists
		requestRegistryMock.On("GetServerSideID", "req-dup").Return("server-dup", nil).Once()
		defer requestRegistryMock.AssertExpectations(t)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		ws.Subscribe(context.Background(), "test", &messageHandlerMock)

		server := httptest.NewServer(ws)
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		// first request succeeds
		assert.NoError(t, client.WriteJSON(domain.Request{ID: "req-dup", Subject: "test"}))

		// allow time for the first request to be processed
		time.Sleep(100 * time.Millisecond)

		// second request with the same ID gets validation error
		assert.NoError(t, client.WriteJSON(domain.Request{ID: "req-dup", Subject: "test"}))

		var response domain.Reply
		assert.NoError(t, client.ReadJSON(&response))
		assert.Equal(t, "req-dup", response.RequestID)
		assert.JSONEq(t, `{"validationErrors":{"request_id":"request_already_exists"}}`, string(response.Payload))
	})

	t.Run("close is idempotent", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
		)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)

		assert.NoError(t, ws.Close())
		assert.NoError(t, ws.Close())
	})

	t.Run("handles publish error on request forwarding", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		publishErr := errors.New("publish failed")

		publishSubscriberMock.On("Subscribe", mock.Anything, "test", &messageHandlerMock).Return(nil)
		publishSubscriberMock.On("Publish", mock.Anything, "test", mock.Anything).Return(publishErr)
		defer publishSubscriberMock.AssertExpectations(t)

		translatorMock.On("Translate", "error_on_processing_the_request", mock.AnythingOfType("[]func(*translator.Params)")).Return("error_on_processing_the_request")
		defer translatorMock.AssertExpectations(t)

		requestRegistryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		requestRegistryMock.On("Add", "req-1").Return("server-1", nil)
		requestRegistryMock.On("DeleteByServerSideID", "server-1").Return(nil).Maybe()
		defer requestRegistryMock.AssertExpectations(t)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		ws.Subscribe(context.Background(), "test", &messageHandlerMock)

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

	t.Run("handles registry add error", func(t *testing.T) {
		t.Parallel()

		var (
			requestRegistryMock   MockRequestRegistry
			publishSubscriberMock messagingMock.MockPublishSubscriber
			translatorMock        translator.TranslatorMock
			messageHandlerMock    messagingMock.MockMessageHandler
		)

		addErr := errors.New("registry full")

		publishSubscriberMock.On("Subscribe", mock.Anything, "test", &messageHandlerMock).Return(nil)
		defer publishSubscriberMock.AssertExpectations(t)

		translatorMock.On("Translate", "error_on_processing_the_request", mock.AnythingOfType("[]func(*translator.Params)")).Return("error_on_processing_the_request")
		defer translatorMock.AssertExpectations(t)

		requestRegistryMock.On("GetServerSideID", "req-1").Return("", domain.ErrNotExists)
		requestRegistryMock.On("Add", "req-1").Return("", addErr)
		defer requestRegistryMock.AssertExpectations(t)

		ws := NewWebsocket(&requestRegistryMock, &publishSubscriberMock, &translatorMock)
		defer ws.Close()

		ws.Subscribe(context.Background(), "test", &messageHandlerMock)

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
