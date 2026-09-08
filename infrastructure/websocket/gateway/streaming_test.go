package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// cancellations records what a session asked to have cancelled, standing in for
// the announcement a gateway publishes to every replica.
type cancellations struct {
	lock sync.Mutex
	ids  []string
}

func (c *cancellations) cancel(_ context.Context, requestID string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.ids = append(c.ids, requestID)
}

func (c *cancellations) recorded() []string {
	c.lock.Lock()
	defer c.lock.Unlock()

	return append([]string(nil), c.ids...)
}

func TestStreamingSession(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// newStreamingSession builds a session over a real registry, because a
	// stream is only meaningful against ids that behave like ids.
	newStreamingSession := func(
		c Conn,
		registry RequestRegistry,
		producer domain.Producer,
		tr *translator.TranslatorMock,
		cancelled *cancellations,
		consumedSubjects ...string,
	) *session {
		subjects := newSubjects()
		for _, subject := range consumedSubjects {
			subjects.add(subject)
		}

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
			cancelStream: cancelled.cancel,
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

	deliverAll := func(s *session, replies ...*domain.Reply) {
		for _, reply := range replies {
			s.replies <- reply
		}
		close(s.replies)

		s.writeReplies()
	}

	t.Run("chunks keep the request open and an end releases it", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(1)
		serverSideID, err := registry.Add("req-1")
		require.NoError(t, err)

		var cancelled cancellations
		c := newIdleConn()
		s := newStreamingSession(c, registry, &messagingMock.MockProduceConsumer{}, echoTranslator(), &cancelled, "logs")

		deliverAll(s,
			&domain.Reply{RequestID: serverSideID, Kind: domain.ReplyChunk, Payload: []byte(`{"line":"first"}`)},
			&domain.Reply{RequestID: serverSideID, Kind: domain.ReplyChunk, Payload: []byte(`{"line":"second"}`)},
			&domain.Reply{RequestID: serverSideID, Kind: domain.ReplyEOF},
		)

		written := c.written()
		require.Len(t, written, 3)

		for _, reply := range written {
			assert.Equal(t, "req-1", reply.RequestID, "the client only ever sees its own id")
		}

		assert.Equal(t, domain.ReplyChunk, written[0].Kind)
		assert.Equal(t, domain.ReplyChunk, written[1].Kind)
		assert.Equal(t, domain.ReplyEOF, written[2].Kind)

		assert.Equal(t, 0, registry.Len(), "the end of a stream releases its request")
	})

	t.Run("a chunk arriving after the stream ended is not delivered", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(1)
		serverSideID, err := registry.Add("req-1")
		require.NoError(t, err)

		var cancelled cancellations
		c := newIdleConn()
		s := newStreamingSession(c, registry, &messagingMock.MockProduceConsumer{}, echoTranslator(), &cancelled, "logs")

		deliverAll(s,
			&domain.Reply{RequestID: serverSideID, Kind: domain.ReplyEOF},
			&domain.Reply{RequestID: serverSideID, Kind: domain.ReplyChunk, Payload: []byte(`{"line":"late"}`)},
		)

		require.Len(t, c.written(), 1, "nothing is written for a request nobody is waiting on")
	})

	t.Run("a single reply still ends its request, as it always did", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(1)
		serverSideID, err := registry.Add("req-1")
		require.NoError(t, err)

		var cancelled cancellations
		c := newIdleConn()
		s := newStreamingSession(c, registry, &messagingMock.MockProduceConsumer{}, echoTranslator(), &cancelled, "runCode")

		deliverAll(s, &domain.Reply{RequestID: serverSideID, Payload: []byte(`{"ok":true}`)})

		require.Len(t, c.written(), 1)
		assert.Equal(t, domain.ReplyFinal, c.written()[0].Kind)
		assert.Equal(t, 0, registry.Len())
	})

	t.Run("input for an open stream reaches its handler under the stream's id", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(1)
		serverSideID, err := registry.Add("req-1")
		require.NoError(t, err)

		var producerMock messagingMock.MockProduceConsumer
		producerMock.On("Produce", mock.Anything, "websocket_attach.input", mock.MatchedBy(func(payload []byte) bool {
			var data map[string]any
			assert.NoError(t, json.Unmarshal(payload, &data))

			return data["id"] == serverSideID && data["keys"] == "ls\n"
		})).Return(nil).Once()
		defer producerMock.AssertExpectations(t)

		var cancelled cancellations
		c := newFakeConn(domain.Request{
			StreamID: "req-1",
			Subject:  "attach.input",
			Payload:  []byte(`{"keys":"ls\n"}`),
		})

		newStreamingSession(c, registry, &producerMock, echoTranslator(), &cancelled, "attach.input").
			run(context.Background())

		assert.Empty(t, c.written(), "input to a stream is not a question, so it is not answered")
		assert.Equal(t, 1, registry.Len(), "input leaves the stream it belongs to open")
	})

	t.Run("a stream request naming no subject ends the stream", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(1)
		serverSideID, err := registry.Add("req-1")
		require.NoError(t, err)

		var cancelled cancellations
		c := newFakeConn(domain.Request{StreamID: "req-1"})

		newStreamingSession(c, registry, &messagingMock.MockProduceConsumer{}, echoTranslator(), &cancelled, "logs").
			run(context.Background())

		written := c.written()
		require.Len(t, written, 1)
		assert.Equal(t, "req-1", written[0].RequestID)
		assert.Equal(t, domain.ReplyEOF, written[0].Kind)

		assert.Equal(t, []string{serverSideID}, cancelled.recorded(), "the producer is told to stop")
		assert.Equal(t, 0, registry.Len())
	})

	t.Run("a stream this connection never opened is rejected", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(1)

		var cancelled cancellations
		c := newFakeConn(domain.Request{StreamID: "someone-elses-stream"})

		newStreamingSession(c, registry, &messagingMock.MockProduceConsumer{}, echoTranslator(), &cancelled, "logs").
			run(context.Background())

		written := c.written()
		require.Len(t, written, 1)

		var failure failureResponse
		require.NoError(t, json.Unmarshal(written[0].Payload, &failure))
		assert.Equal(t, streamNotOpenMessage, failure.ValidationErrors[streamIDField])

		assert.Empty(t, cancelled.recorded(), "nothing was cancelled for a stream that was never open")
	})

	t.Run("a client that goes away has its open streams cancelled", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(2)
		first, err := registry.Add("req-1")
		require.NoError(t, err)
		second, err := registry.Add("req-2")
		require.NoError(t, err)

		var cancelled cancellations
		c := newFakeConn()

		newStreamingSession(c, registry, &messagingMock.MockProduceConsumer{}, echoTranslator(), &cancelled, "logs").
			run(context.Background())

		assert.ElementsMatch(t, []string{first, second}, cancelled.recorded())
	})
}

func TestStreams(t *testing.T) {
	t.Parallel()

	t.Run("cancels the stream it is told about", func(t *testing.T) {
		t.Parallel()

		streams := NewStreams()

		ctx, cancel := context.WithCancel(context.Background())
		streams.Add("server-1", cancel)

		payload, err := json.Marshal(&StreamCancelled{RequestID: "server-1"})
		require.NoError(t, err)

		require.NoError(t, streams.Handle(context.Background(), payload))

		<-ctx.Done()
		assert.Equal(t, 0, streams.Len())
	})

	t.Run("leaves another replica's stream alone", func(t *testing.T) {
		t.Parallel()

		streams := NewStreams()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		streams.Add("server-1", cancel)

		payload, err := json.Marshal(&StreamCancelled{RequestID: "server-2"})
		require.NoError(t, err)

		require.NoError(t, streams.Handle(context.Background(), payload))

		assert.NoError(t, ctx.Err())
		assert.Equal(t, 1, streams.Len())
	})

	t.Run("a stream that ended on its own is forgotten", func(t *testing.T) {
		t.Parallel()

		streams := NewStreams()

		streams.Add("server-1", func() {})
		streams.Remove("server-1")

		assert.False(t, streams.Cancel("server-1"))
		assert.Equal(t, 0, streams.Len())
	})

	t.Run("registering a request twice cancels the older stream", func(t *testing.T) {
		t.Parallel()

		streams := NewStreams()

		older, cancelOlder := context.WithCancel(context.Background())
		streams.Add("server-1", cancelOlder)

		newer, cancelNewer := context.WithCancel(context.Background())
		defer cancelNewer()
		streams.Add("server-1", cancelNewer)

		<-older.Done()
		assert.NoError(t, newer.Err())
	})

	t.Run("a malformed cancellation is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		streams := NewStreams()

		assert.NoError(t, streams.Handle(context.Background(), []byte("{")))
	})
}
