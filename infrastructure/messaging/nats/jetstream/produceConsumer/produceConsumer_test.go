package produceConsumer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
)

// jetStream is a NATS server with JetStream of the test's own, and a
// connection to it.
func jetStream(t *testing.T) *nats.Conn {
	t.Helper()

	natsServer, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	require.NoError(t, err)

	go natsServer.Start()
	t.Cleanup(natsServer.Shutdown)

	require.True(t, natsServer.ReadyForConnections(5*time.Second), "nats did not come up")

	connection, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	t.Cleanup(connection.Close)

	return connection
}

func TestProduceConsumer_Options(t *testing.T) {
	t.Parallel()

	t.Run("messages of one subject are handled several at once, no more than allowed", func(t *testing.T) {
		t.Parallel()

		pc, err := NewProduceConsumer(jetStream(t), "node-1", slog.New(slog.DiscardHandler), WithConcurrency(3))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer func() {
			cancel()
			pc.Wait()
		}()

		var inside, most, handled atomic.Int32

		require.NoError(t, pc.Consume(ctx, "workloadTest", domain.MessageHandlerFunc(func(context.Context, []byte) error {
			now := inside.Add(1)
			for {
				seen := most.Load()
				if now <= seen || most.CompareAndSwap(seen, now) {
					break
				}
			}

			time.Sleep(100 * time.Millisecond)
			inside.Add(-1)
			handled.Add(1)

			return nil
		})))

		for range 9 {
			require.NoError(t, pc.Produce(t.Context(), "workloadTest", []byte("{}")))
		}

		require.Eventually(t, func() bool { return handled.Load() == 9 }, 10*time.Second, 10*time.Millisecond)
		assert.Equal(t, int32(3), most.Load())
	})

	t.Run("a message handled for longer than the ack wait is not handed over again meanwhile", func(t *testing.T) {
		t.Parallel()

		pc, err := NewProduceConsumer(jetStream(t), "node-1", slog.New(slog.DiscardHandler),
			WithAckWait(400*time.Millisecond),
			WithProgress(100*time.Millisecond),
			WithConcurrency(2),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer func() {
			cancel()
			pc.Wait()
		}()

		var deliveries atomic.Int32

		require.NoError(t, pc.Consume(ctx, "workloadLong", domain.MessageHandlerFunc(func(context.Context, []byte) error {
			deliveries.Add(1)
			time.Sleep(1200 * time.Millisecond)

			return nil
		})))

		require.NoError(t, pc.Produce(t.Context(), "workloadLong", []byte("{}")))

		require.Eventually(t, func() bool { return deliveries.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)

		// past the handler's end and several ack waits more.
		time.Sleep(2 * time.Second)

		assert.Equal(t, int32(1), deliveries.Load())
	})
}

// TestProduceConsumer_failed holds a message its handler failed to being
// handed over again: at once, or once as long as the handler asked for has
// passed (domain.Later).
func TestProduceConsumer_failed(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		failure error
		after   time.Duration
	}{
		"at once, for a failure that asks nothing of when": {failure: errors.New("the database is gone")},
		"and once its handler's while has passed, for one that asks for it later": {
			failure: &domain.Later{Err: errors.New("the database is gone"), After: 700 * time.Millisecond},
			after:   700 * time.Millisecond,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pc, err := NewProduceConsumer(jetStream(t), "node-1", slog.New(slog.DiscardHandler))
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(t.Context())
			defer func() {
				cancel()
				pc.Wait()
			}()

			var (
				lock       sync.Mutex
				deliveries []time.Time
			)

			require.NoError(t, pc.Consume(ctx, "workloadFailed", domain.MessageHandlerFunc(func(context.Context, []byte) error {
				lock.Lock()
				defer lock.Unlock()

				deliveries = append(deliveries, time.Now())
				if len(deliveries) == 1 {
					return tt.failure
				}

				return nil
			})))

			require.NoError(t, pc.Produce(t.Context(), "workloadFailed", []byte("{}")))

			require.Eventually(t, func() bool {
				lock.Lock()
				defer lock.Unlock()

				return len(deliveries) >= 2
			}, 10*time.Second, 10*time.Millisecond, "handed over again")

			lock.Lock()
			defer lock.Unlock()

			waited := deliveries[1].Sub(deliveries[0])
			if tt.after > 0 {
				assert.GreaterOrEqual(t, waited, tt.after-50*time.Millisecond, "not before its while has passed")
			} else {
				assert.Less(t, waited, 500*time.Millisecond, "at once")
			}
		})
	}
}
