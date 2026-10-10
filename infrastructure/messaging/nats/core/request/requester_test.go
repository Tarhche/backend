package request

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// connect runs a NATS server of its own for one test and connects to it.
func connect(t *testing.T) *nats.Conn {
	t.Helper()

	natsServer, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: server.RANDOM_PORT, NoLog: true, NoSigs: true})
	require.NoError(t, err)

	go natsServer.Start()
	t.Cleanup(natsServer.Shutdown)

	require.True(t, natsServer.ReadyForConnections(5*time.Second), "the nats server did not come up")

	connection, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	t.Cleanup(connection.Close)

	return connection
}

// answer stands in for a node: whatever is asked on its subject is answered
// by answering. An answer nobody waits for any more is dropped, as a node's is.
func answer(t *testing.T, connection *nats.Conn, nodeName string, answering func(request noderequest.Request) noderequest.Reply) {
	t.Helper()

	subscription, err := connection.Subscribe(noderequest.Subject(nodeName), func(msg *nats.Msg) {
		var request noderequest.Request
		if err := json.Unmarshal(msg.Data, &request); err != nil {
			return
		}

		payload, err := json.Marshal(answering(request))
		if err != nil {
			return
		}

		_ = msg.Respond(payload)
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	require.NoError(t, connection.Flush())
}

func TestRequester_Request(t *testing.T) {
	t.Parallel()

	t.Run("a node's answer is what it replied", func(t *testing.T) {
		t.Parallel()

		connection := connect(t)

		var asked noderequest.Request
		answer(t, connection, "workload-orchestrator-01", func(request noderequest.Request) noderequest.Reply {
			asked = request

			return noderequest.Reply{OK: true, Result: json.RawMessage(`{"lines":[]}`), Truncated: true}
		})

		reply, err := NewRequester(connection, time.Second).Request(context.Background(), "workload-orchestrator-01", noderequest.Request{
			Op:      "container.logs",
			VMUUID:  "container-uuid",
			Payload: json.RawMessage(`{"tail":20}`),
		})
		require.NoError(t, err)

		assert.True(t, reply.OK)
		assert.JSONEq(t, `{"lines":[]}`, string(reply.Result))
		assert.True(t, reply.Truncated)

		assert.Equal(t, noderequest.Op("container.logs"), asked.Op)
		assert.Equal(t, "container-uuid", asked.VMUUID)
		assert.JSONEq(t, `{"tail":20}`, string(asked.Payload))
	})

	t.Run("a node's refusal is in the reply, not an error", func(t *testing.T) {
		t.Parallel()

		connection := connect(t)

		answer(t, connection, "workload-orchestrator-01", func(noderequest.Request) noderequest.Reply {
			return noderequest.Failed(vm.ErrNotRunning)
		})

		reply, err := NewRequester(connection, time.Second).Request(context.Background(), "workload-orchestrator-01", noderequest.Request{Op: "vm.logs", VMUUID: "vm-uuid"})
		require.NoError(t, err)

		assert.False(t, reply.OK)
		assert.ErrorIs(t, reply.Err(), vm.ErrNotRunning)
	})

	t.Run("a node nobody is answering for is told apart", func(t *testing.T) {
		t.Parallel()

		connection := connect(t)

		_, err := NewRequester(connection, time.Second).Request(context.Background(), "workload-orchestrator-09", noderequest.Request{Op: "vm.logs"})

		assert.ErrorIs(t, err, ErrNoNode)
		assert.False(t, errors.Is(err, domain.ErrNotExists))
	})

	t.Run("a node that does not answer in time is a deadline", func(t *testing.T) {
		t.Parallel()

		connection := connect(t)

		answer(t, connection, "workload-orchestrator-01", func(noderequest.Request) noderequest.Reply {
			time.Sleep(500 * time.Millisecond)

			return noderequest.Reply{OK: true}
		})

		started := time.Now()
		_, err := NewRequester(connection, 50*time.Millisecond).Request(context.Background(), "workload-orchestrator-01", noderequest.Request{Op: "container.stats"})

		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(started), 400*time.Millisecond, "it is given the request timeout")
	})

	t.Run("a caller may give up sooner", func(t *testing.T) {
		t.Parallel()

		connection := connect(t)

		answer(t, connection, "workload-orchestrator-01", func(noderequest.Request) noderequest.Reply {
			time.Sleep(time.Second)

			return noderequest.Reply{OK: true}
		})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := NewRequester(connection, time.Minute).Request(ctx, "workload-orchestrator-01", noderequest.Request{Op: "container.logs"})
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
