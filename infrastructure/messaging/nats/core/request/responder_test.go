package request

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// responderNATS is a NATS server of the test's own, and a connection to it.
func responderNATS(t *testing.T) *nats.Conn {
	t.Helper()

	natsServer, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	require.NoError(t, err)

	go natsServer.Start()
	t.Cleanup(natsServer.Shutdown)

	require.True(t, natsServer.ReadyForConnections(5*time.Second), "nats did not come up")

	connection, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	t.Cleanup(connection.Close)

	return connection
}

// handlerFunc answers requests with a function.
type handlerFunc func(ctx context.Context, request noderequest.Request) noderequest.Reply

func (f handlerFunc) Handle(ctx context.Context, request noderequest.Request) noderequest.Reply {
	return f(ctx, request)
}

func ask(t *testing.T, connection *nats.Conn, request noderequest.Request, timeout time.Duration) (noderequest.Reply, error) {
	t.Helper()

	payload, err := json.Marshal(request)
	require.NoError(t, err)

	message, err := connection.Request(noderequest.Subject("workload-orchestrator-01"), payload, timeout)
	if err != nil {
		return noderequest.Reply{}, err
	}

	var reply noderequest.Reply
	require.NoError(t, json.Unmarshal(message.Data, &reply))

	return reply, nil
}

func serve(t *testing.T, connection *nats.Conn, handler noderequest.Handler, options ResponderOptions) *Responder {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())

	responder := NewResponder(connection, handler, options, slog.New(slog.DiscardHandler))
	require.NoError(t, responder.Serve(ctx, noderequest.Subject("workload-orchestrator-01")))
	require.NoError(t, connection.Flush())

	t.Cleanup(func() {
		cancel()
		responder.Wait()
	})

	return responder
}

func TestResponder(t *testing.T) {
	t.Parallel()

	t.Run("a request is answered with what the node answers", func(t *testing.T) {
		t.Parallel()

		connection := responderNATS(t)

		serve(t, connection, handlerFunc(func(_ context.Context, request noderequest.Request) noderequest.Reply {
			result, _ := json.Marshal(map[string]string{"asked": string(request.Op), "of": request.VMUUID})

			return noderequest.Reply{OK: true, Result: result}
		}), ResponderOptions{Concurrency: 2, Timeout: time.Second})

		reply, err := ask(t, connection, noderequest.Request{Op: "container.logs", VMUUID: "c-1"}, 5*time.Second)
		require.NoError(t, err)

		assert.True(t, reply.OK)
		assert.JSONEq(t, `{"asked":"container.logs","of":"c-1"}`, string(reply.Result))
	})

	t.Run("a request that cannot be read is refused", func(t *testing.T) {
		t.Parallel()

		connection := responderNATS(t)

		serve(t, connection, handlerFunc(func(context.Context, noderequest.Request) noderequest.Reply {
			t.Error("nothing unreadable is handled")

			return noderequest.Reply{}
		}), ResponderOptions{Concurrency: 1, Timeout: time.Second})

		message, err := connection.Request(noderequest.Subject("workload-orchestrator-01"), []byte("{"), 5*time.Second)
		require.NoError(t, err)

		var reply noderequest.Reply
		require.NoError(t, json.Unmarshal(message.Data, &reply))

		assert.False(t, reply.OK)
		assert.Equal(t, noderequest.CodeInvalid, reply.Error.Code)
	})

	t.Run("a request is given the timeout to be answered in", func(t *testing.T) {
		t.Parallel()

		connection := responderNATS(t)

		given := make(chan time.Duration, 1)

		serve(t, connection, handlerFunc(func(ctx context.Context, request noderequest.Request) noderequest.Reply {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)

			given <- time.Until(deadline)

			return noderequest.Reply{OK: true}
		}), ResponderOptions{Concurrency: 2, Timeout: 30 * time.Second})

		_, err := ask(t, connection, noderequest.Request{Op: "container.stop", VMUUID: "c-1"}, 5*time.Second)
		require.NoError(t, err)

		assert.InDelta(t, 30*time.Second, <-given, float64(5*time.Second))
	})

	t.Run("a caller giving up does not cut short what it asked for", func(t *testing.T) {
		t.Parallel()

		connection := responderNATS(t)

		finished := make(chan error, 1)

		serve(t, connection, handlerFunc(func(ctx context.Context, _ noderequest.Request) noderequest.Reply {
			select {
			case <-time.After(300 * time.Millisecond):
				finished <- nil
			case <-ctx.Done():
				finished <- ctx.Err()
			}

			return noderequest.Reply{OK: true}
		}), ResponderOptions{Concurrency: 1, Timeout: time.Minute})

		_, err := ask(t, connection, noderequest.Request{Op: "container.stop", VMUUID: "c-1"}, 50*time.Millisecond)
		require.ErrorIs(t, err, nats.ErrTimeout, "the caller gave up")

		select {
		case err := <-finished:
			assert.NoError(t, err, "the stop went on to the end")
		case <-time.After(5 * time.Second):
			t.Fatal("the stop never finished")
		}
	})

	t.Run("no more requests are answered at once than allowed", func(t *testing.T) {
		t.Parallel()

		connection := responderNATS(t)

		var inside, most atomic.Int32

		serve(t, connection, handlerFunc(func(context.Context, noderequest.Request) noderequest.Reply {
			now := inside.Add(1)
			for {
				seen := most.Load()
				if now <= seen || most.CompareAndSwap(seen, now) {
					break
				}
			}

			time.Sleep(50 * time.Millisecond)
			inside.Add(-1)

			return noderequest.Reply{OK: true}
		}), ResponderOptions{Concurrency: 3, Timeout: time.Minute})

		var wg sync.WaitGroup
		for range 12 {
			wg.Go(func() {
				reply, err := ask(t, connection, noderequest.Request{Op: "container.logs", VMUUID: "c-1"}, 10*time.Second)
				assert.NoError(t, err)
				assert.True(t, reply.OK)
			})
		}

		wg.Wait()

		assert.Equal(t, int32(3), most.Load(), "the requests were answered three at a time")
	})

	t.Run("an answer too large for one message is an error rather than nothing", func(t *testing.T) {
		t.Parallel()

		connection := responderNATS(t)

		serve(t, connection, handlerFunc(func(context.Context, noderequest.Request) noderequest.Reply {
			huge, _ := json.Marshal(make([]byte, noderequest.MaxReplyBytes))

			return noderequest.Reply{OK: true, Result: huge}
		}), ResponderOptions{Concurrency: 1, Timeout: time.Minute})

		reply, err := ask(t, connection, noderequest.Request{Op: "container.logs", VMUUID: "c-1"}, 5*time.Second)
		require.NoError(t, err)

		assert.False(t, reply.OK)
		assert.Equal(t, noderequest.CodeInternal, reply.Error.Code)
	})
}
