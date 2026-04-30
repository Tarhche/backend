package websocket

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/cache"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCacheDecorator(t *testing.T) {
	t.Parallel()

	const cachedSubject = "run-code"

	t.Run("consume passes through subjects not in allowlist", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock             MockWebsocket
			cacheMock          cache.MockCache
			messageHandlerMock messagingMock.MockMessageHandler
		)

		// for non-allowlisted subjects the handler must be forwarded unwrapped
		wsMock.On("Consume", mock.Anything, "other-subject", &messageHandlerMock).Return(nil)
		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		assert.NoError(t, decorator.Consume(context.Background(), "other-subject", &messageHandlerMock))
	})

	t.Run("consume returns error from underlying consumer", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock             MockWebsocket
			cacheMock          cache.MockCache
			messageHandlerMock messagingMock.MockMessageHandler
		)

		expectedErr := errors.New("consume failed")
		wsMock.On("Consume", mock.Anything, cachedSubject, mock.Anything).Return(expectedErr)
		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		err := decorator.Consume(context.Background(), cachedSubject, &messageHandlerMock)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("cache hit replies immediately and skips handler", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock             MockWebsocket
			cacheMock          cache.MockCache
			messageHandlerMock messagingMock.MockMessageHandler
		)

		payload := []byte(`{"id":"server-1","data":"hello"}`)
		cachedReply := []byte(`{"result":"hi"}`)

		var wrappedHandler domain.MessageHandler
		wsMock.On("Consume", mock.Anything, cachedSubject, mock.Anything).
			Run(func(args mock.Arguments) {
				wrappedHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)

		cacheMock.On("Get", mock.AnythingOfType("string")).Return(cachedReply, nil)

		wsMock.On("Reply", mock.Anything, mock.MatchedBy(func(r *domain.Reply) bool {
			return r.RequestID == "server-1" && string(r.Payload) == string(cachedReply)
		})).Return(nil)

		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)
		defer messageHandlerMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		assert.NoError(t, decorator.Consume(context.Background(), cachedSubject, &messageHandlerMock))
		assert.NoError(t, wrappedHandler.Handle(payload))
	})

	t.Run("cache miss forwards request to handler", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock             MockWebsocket
			cacheMock          cache.MockCache
			messageHandlerMock messagingMock.MockMessageHandler
		)

		payload := []byte(`{"id":"server-1","data":"hello"}`)

		var wrappedHandler domain.MessageHandler
		wsMock.On("Consume", mock.Anything, cachedSubject, mock.Anything).
			Run(func(args mock.Arguments) {
				wrappedHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)

		cacheMock.On("Get", mock.AnythingOfType("string")).Return([]byte(nil), domain.ErrNotExists)
		cacheMock.On("Set", "pending.server-1", mock.AnythingOfType("[]uint8")).Return(nil)
		messageHandlerMock.On("Handle", payload).Return(nil)

		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)
		defer messageHandlerMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		assert.NoError(t, decorator.Consume(context.Background(), cachedSubject, &messageHandlerMock))
		assert.NoError(t, wrappedHandler.Handle(payload))
	})

	t.Run("reply stores payload in cache after a miss", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock             MockWebsocket
			cacheMock          cache.MockCache
			messageHandlerMock messagingMock.MockMessageHandler
		)

		payload := []byte(`{"id":"server-1","data":"hello"}`)
		replyPayload := []byte(`{"result":"hi"}`)

		checksum, _, err := payloadChecksum(payload)
		assert.NoError(t, err)

		var wrappedHandler domain.MessageHandler

		wsMock.On("Consume", mock.Anything, cachedSubject, mock.Anything).
			Run(func(args mock.Arguments) {
				wrappedHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)

		// checksum lookup on Consume — miss
		cacheMock.On("Get", "cached."+checksum).Return([]byte(nil), domain.ErrNotExists).Once()
		// pending entry written so a different replica can resolve the reply
		cacheMock.On("Set", "pending.server-1", []byte(checksum)).Return(nil)
		messageHandlerMock.On("Handle", payload).Return(nil)

		// pending lookup on Reply — returns the checksum the Consume side stored
		cacheMock.On("Get", "pending.server-1").Return([]byte(checksum), nil).Once()
		cacheMock.On("Set", "cached."+checksum, replyPayload).Return(nil)
		cacheMock.On("Purge", "pending.server-1").Return(nil)

		wsMock.On("Reply", mock.Anything, mock.MatchedBy(func(r *domain.Reply) bool {
			return r.RequestID == "server-1" && string(r.Payload) == string(replyPayload)
		})).Return(nil)

		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)
		defer messageHandlerMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		assert.NoError(t, decorator.Consume(context.Background(), cachedSubject, &messageHandlerMock))
		assert.NoError(t, wrappedHandler.Handle(payload))

		assert.NoError(t, decorator.Reply(context.Background(), &domain.Reply{
			RequestID: "server-1",
			Payload:   replyPayload,
		}))
	})

	t.Run("reply without pending entry skips cache write", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock    MockWebsocket
			cacheMock cache.MockCache
		)

		reply := &domain.Reply{RequestID: "unknown", Payload: []byte("data")}
		cacheMock.On("Get", "pending.unknown").Return([]byte(nil), domain.ErrNotExists)
		wsMock.On("Reply", mock.Anything, reply).Return(nil)
		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		assert.NoError(t, decorator.Reply(context.Background(), reply))
	})

	t.Run("reply propagates parent reply error", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock    MockWebsocket
			cacheMock cache.MockCache
		)

		expectedErr := errors.New("reply failed")
		reply := &domain.Reply{RequestID: "unknown", Payload: []byte("data")}
		cacheMock.On("Get", "pending.unknown").Return([]byte(nil), domain.ErrNotExists)
		wsMock.On("Reply", mock.Anything, reply).Return(expectedErr)
		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		err := decorator.Reply(context.Background(), reply)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("reply still completes when cache set fails", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock             MockWebsocket
			cacheMock          cache.MockCache
			messageHandlerMock messagingMock.MockMessageHandler
		)

		payload := []byte(`{"id":"server-1","data":"hello"}`)
		replyPayload := []byte(`{"result":"hi"}`)

		checksum, _, err := payloadChecksum(payload)
		assert.NoError(t, err)

		var wrappedHandler domain.MessageHandler
		wsMock.On("Consume", mock.Anything, cachedSubject, mock.Anything).
			Run(func(args mock.Arguments) {
				wrappedHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)

		cacheMock.On("Get", "cached."+checksum).Return([]byte(nil), domain.ErrNotExists).Once()
		cacheMock.On("Set", "pending.server-1", []byte(checksum)).Return(nil)
		messageHandlerMock.On("Handle", payload).Return(nil)
		cacheMock.On("Get", "pending.server-1").Return([]byte(checksum), nil).Once()
		cacheMock.On("Set", "cached."+checksum, replyPayload).Return(errors.New("cache down"))
		cacheMock.On("Purge", "pending.server-1").Return(nil)
		wsMock.On("Reply", mock.Anything, mock.Anything).Return(nil)

		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)
		defer messageHandlerMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		assert.NoError(t, decorator.Consume(context.Background(), cachedSubject, &messageHandlerMock))
		assert.NoError(t, wrappedHandler.Handle(payload))

		assert.NoError(t, decorator.Reply(context.Background(), &domain.Reply{
			RequestID: "server-1",
			Payload:   replyPayload,
		}))
	})

	t.Run("consume falls through to handler when payload checksum fails", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock             MockWebsocket
			cacheMock          cache.MockCache
			messageHandlerMock messagingMock.MockMessageHandler
		)

		// payload missing the required string "id" field — payloadChecksum errors
		invalidPayload := []byte(`{"data":"hello"}`)

		var wrappedHandler domain.MessageHandler
		wsMock.On("Consume", mock.Anything, cachedSubject, mock.Anything).
			Run(func(args mock.Arguments) {
				wrappedHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)
		messageHandlerMock.On("Handle", invalidPayload).Return(nil)

		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t) // no cache calls expected
		defer messageHandlerMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		assert.NoError(t, decorator.Consume(context.Background(), cachedSubject, &messageHandlerMock))
		assert.NoError(t, wrappedHandler.Handle(invalidPayload))
	})

	t.Run("checksum is stable across different injected ids", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock             MockWebsocket
			cacheMock          cache.MockCache
			messageHandlerMock messagingMock.MockMessageHandler
		)

		// same business payload, different server-side ids
		first := []byte(`{"id":"server-1","data":"hello"}`)
		second := []byte(`{"id":"server-2","data":"hello"}`)
		cachedReply := []byte(`{"result":"hi"}`)

		var (
			wrappedHandler domain.MessageHandler
			firstKey       string
		)

		wsMock.On("Consume", mock.Anything, cachedSubject, mock.Anything).
			Run(func(args mock.Arguments) {
				wrappedHandler = args.Get(2).(domain.MessageHandler)
			}).Return(nil)

		// first invocation: miss; pending entry stored under server-1
		cacheMock.On("Get", mock.MatchedBy(func(key string) bool {
			return key != "pending.server-1" && key != "pending.server-2"
		})).Run(func(args mock.Arguments) {
			firstKey = args.String(0)
		}).Return([]byte(nil), domain.ErrNotExists).Once()
		cacheMock.On("Set", "pending.server-1", mock.MatchedBy(func(value []byte) bool {
			// Set on pending stores the bare checksum; the cache lookup key prefixes it with "cached."
			return "cached."+string(value) == firstKey
		})).Return(nil)
		messageHandlerMock.On("Handle", first).Return(nil)

		// second invocation: must look up the same key as the first
		cacheMock.On("Get", mock.MatchedBy(func(key string) bool {
			return key == firstKey
		})).Return(cachedReply, nil).Once()
		wsMock.On("Reply", mock.Anything, mock.MatchedBy(func(r *domain.Reply) bool {
			return r.RequestID == "server-2" && string(r.Payload) == string(cachedReply)
		})).Return(nil)

		defer wsMock.AssertExpectations(t)
		defer cacheMock.AssertExpectations(t)
		defer messageHandlerMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)

		assert.NoError(t, decorator.Consume(context.Background(), cachedSubject, &messageHandlerMock))
		assert.NoError(t, wrappedHandler.Handle(first))
		assert.NoError(t, wrappedHandler.Handle(second))
	})

	t.Run("serve http delegates to parent", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock    MockWebsocket
			cacheMock cache.MockCache
		)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "/", nil)

		wsMock.On("ServeHTTP", mock.Anything, mock.Anything).Return()
		defer wsMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)
		decorator.ServeHTTP(recorder, request)
	})

	t.Run("close delegates to parent", func(t *testing.T) {
		t.Parallel()

		var (
			wsMock    MockWebsocket
			cacheMock cache.MockCache
		)

		expectedErr := errors.New("close failed")
		wsMock.On("Close").Return(expectedErr)
		defer wsMock.AssertExpectations(t)

		decorator := NewCacheDecorator(&wsMock, &cacheMock, cachedSubject)
		assert.ErrorIs(t, decorator.Close(), expectedErr)
	})
}
