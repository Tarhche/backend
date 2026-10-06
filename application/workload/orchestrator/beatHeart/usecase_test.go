package beatHeart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
)

// logs is a log a test can read back, written from wherever.
type logs struct {
	lock    sync.Mutex
	written bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	return l.written.Write(p)
}

func (l *logs) String() string {
	l.lock.Lock()
	defer l.lock.Unlock()

	return l.written.String()
}

// a node is what a heartbeat is beaten for: what it offers, where it says
// so, and what it logs.
type aNode struct {
	heartbeat *UseCase
	recorder  *messaging.Recorder
	logs      *logs
}

func beating(t *testing.T, kinds *kind.Registry[kind.NodeBinding], stateTimeout time.Duration) *aNode {
	t.Helper()

	var nodeManager runtime.MockNodeManager
	nodeManager.On("Stats", mock.Anything, "node-1").Return(node.Stats{PIDs: 7}, nil)
	nodeManager.On("Capacity", mock.Anything).Return(vm.Info{Engine: "memory", CPUs: 8, Memory: 16 << 30, Disk: 100 << 30, Allocated: vm.Resources{CPUs: 2}}, nil)

	n := &aNode{recorder: &messaging.Recorder{}, logs: &logs{}}
	n.heartbeat = NewUseCase(n.recorder, &nodeManager, kinds, stateTimeout, "node-1", slog.New(slog.NewTextHandler(n.logs, nil)))

	return n
}

// beat beats once, and is the heartbeat it sent and that heartbeat's fields
// as they were written.
func (n *aNode) beat(t *testing.T) (events.Heartbeat, map[string]json.RawMessage) {
	t.Helper()

	n.recorder.Reset()
	require.NoError(t, n.heartbeat.Execute(t.Context()))

	messages := n.recorder.Messages()
	require.Len(t, messages, 1)
	require.Equal(t, events.HeartbeatName, messages[0].Subject)

	var heartbeat events.Heartbeat
	require.NoError(t, json.Unmarshal(messages[0].Payload, &heartbeat))

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(messages[0].Payload, &fields))

	return heartbeat, fields
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	lamp := kind.Observed[lampStatus]{
		UUID:   "lamp-1",
		Owners: []kind.Reference{{Kind: "room", UUID: "room-1"}},
		Status: lampStatus{Status: kind.Status{State: lit}, Brightness: 80},
	}

	t.Run("a node that runs no kinds beats as it always did", func(t *testing.T) {
		t.Parallel()

		for name, kinds := range map[string]*kind.Registry[kind.NodeBinding]{
			"none registered": kind.NewRegistry[kind.NodeBinding](),
			"no registry":     nil,
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				heartbeat, fields := beating(t, kinds, time.Second).beat(t)

				assert.Equal(t, "node-1", heartbeat.Name)
				assert.Equal(t, node.OrchestratorRole, heartbeat.Role)
				assert.Equal(t, node.Stats{PIDs: 7}, heartbeat.Stats)
				assert.Equal(t, vm.Info{Engine: "memory", CPUs: 8, Memory: 16 << 30, Disk: 100 << 30, Allocated: vm.Resources{CPUs: 2}}, heartbeat.Capacity, "what it offers to VMs, which they are placed by")
				assert.WithinDuration(t, time.Now(), heartbeat.At, time.Minute)

				assert.ElementsMatch(t, []string{"Name", "Role", "Stats", "Capacity", "At"}, keys(fields))
			})
		}
	})

	for name, tt := range map[string]struct {
		states map[string]func(context.Context) (kind.Report[lampStatus], error)
		want   map[string]string
		logs   []string
	}{
		"every kind's report is in the beat, by kind, with the parents it could not look inside": {
			states: map[string]func(context.Context) (kind.Report[lampStatus], error){
				"lamp":   holding(kind.Report[lampStatus]{Instances: []kind.Observed[lampStatus]{lamp}, Unseen: []string{"room-2"}}),
				"kettle": holding(kind.Report[lampStatus]{}),
			},
			want: map[string]string{
				"lamp":   `{"instances":[{"kind":"lamp","uuid":"lamp-1","owners":[{"kind":"room","uuid":"room-1"}],"status":{"state":"lit","brightness":80}}],"unseen":["room-2"]}`,
				"kettle": `{"instances":[]}`,
			},
		},
		"a kind that could not look is left out, and never reported holding nothing": {
			states: map[string]func(context.Context) (kind.Report[lampStatus], error){
				"lamp": func(context.Context) (kind.Report[lampStatus], error) {
					return kind.Report[lampStatus]{}, errors.New("the fuse box is locked")
				},
				"kettle": holding(kind.Report[lampStatus]{}),
			},
			want: map[string]string{"kettle": `{"instances":[]}`},
			logs: []string{"kind=lamp", "the fuse box is locked"},
		},
		"a kind that takes too long is left out, and holds up neither the beat nor the others": {
			states: map[string]func(context.Context) (kind.Report[lampStatus], error){
				"lamp": func(ctx context.Context) (kind.Report[lampStatus], error) {
					<-ctx.Done()

					return kind.Report[lampStatus]{}, ctx.Err()
				},
				"kettle": holding(kind.Report[lampStatus]{Instances: []kind.Observed[lampStatus]{lamp}}),
			},
			want: map[string]string{"kettle": `{"instances":[{"kind":"kettle","uuid":"lamp-1","owners":[{"kind":"room","uuid":"room-1"}],"status":{"state":"lit","brightness":80}}]}`},
			logs: []string{"kind=lamp"},
		},
		"and when no kind could say anything, the beat goes out all the same": {
			states: map[string]func(context.Context) (kind.Report[lampStatus], error){
				"lamp": func(context.Context) (kind.Report[lampStatus], error) {
					return kind.Report[lampStatus]{}, errors.New("the fuse box is locked")
				},
			},
			logs: []string{"kind=lamp", "the fuse box is locked"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			n := beating(t, registered(t, tt.states), 100*time.Millisecond)

			started := time.Now()
			heartbeat, fields := n.beat(t)

			assert.Less(t, time.Since(started), 5*time.Second, "the beat waits for nobody longer than it gives them")
			assert.Equal(t, node.Stats{PIDs: 7}, heartbeat.Stats, "what the node offers is said whatever the kinds said")

			observed := make(map[string]string, len(heartbeat.Observations))
			for name, report := range heartbeat.Observations {
				encoded, err := json.Marshal(report)
				require.NoError(t, err)

				observed[name] = string(encoded)
			}

			require.Len(t, observed, len(tt.want))
			for name, report := range tt.want {
				assert.JSONEq(t, report, observed[name], name)
			}

			if len(tt.want) == 0 {
				assert.NotContains(t, fields, "observations", "a beat with nothing observed says nothing of it")
			}

			for _, said := range tt.logs {
				assert.Contains(t, n.logs.String(), said)
			}
		})
	}

	t.Run("a kind whose state the control plane knows is not asked", func(t *testing.T) {
		t.Parallel()

		var asked atomic.Int32

		kinds := kind.NewRegistry[kind.NodeBinding]()
		require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lampNamed("receipt", kind.OnControlPlane), &lamps{
			state: func(context.Context) (kind.Report[lampStatus], error) {
				asked.Add(1)

				return kind.Report[lampStatus]{}, nil
			},
		})))

		heartbeat, _ := beating(t, kinds, time.Second).beat(t)

		assert.Empty(t, heartbeat.Observations)
		assert.Zero(t, asked.Load())
	})

	t.Run("many kinds are asked at once", func(t *testing.T) {
		t.Parallel()

		const many = 8

		// each kind answers only once every one of them has been asked, which
		// kinds asked one after another never are.
		var arrived sync.WaitGroup
		arrived.Add(many)

		together := func(ctx context.Context) (kind.Report[lampStatus], error) {
			arrived.Done()

			all := make(chan struct{})
			go func() {
				arrived.Wait()
				close(all)
			}()

			select {
			case <-all:
				return kind.Report[lampStatus]{}, nil
			case <-ctx.Done():
				return kind.Report[lampStatus]{}, ctx.Err()
			}
		}

		states := make(map[string]func(context.Context) (kind.Report[lampStatus], error), many)
		for i := range many {
			states[fmt.Sprintf("lamp%d", i)] = together
		}

		heartbeat, _ := beating(t, registered(t, states), 10*time.Second).beat(t)

		assert.Len(t, heartbeat.Observations, many)
	})

	t.Run("a kind that does not let go is asked once, not once a beat", func(t *testing.T) {
		t.Parallel()

		var asked atomic.Int32
		letGo := make(chan struct{})

		stuck := func(context.Context) (kind.Report[lampStatus], error) {
			if asked.Add(1) == 1 {
				// it does not listen to being given up on.
				<-letGo
			}

			return kind.Report[lampStatus]{}, nil
		}

		n := beating(t, registered(t, map[string]func(context.Context) (kind.Report[lampStatus], error){"lamp": stuck}), 20*time.Millisecond)

		heartbeat, _ := n.beat(t)
		assert.Empty(t, heartbeat.Observations, "it did not answer in time")

		heartbeat, _ = n.beat(t)
		assert.Empty(t, heartbeat.Observations, "it is still answering the last beat")
		assert.Equal(t, int32(1), asked.Load(), "and is not asked again meanwhile")
		assert.Contains(t, n.logs.String(), "still saying what it holds")

		close(letGo)

		require.Eventually(t, func() bool {
			n.recorder.Reset()

			if err := n.heartbeat.Execute(context.Background()); err != nil {
				return false
			}

			var heartbeat events.Heartbeat

			return n.recorder.Last(events.HeartbeatName, &heartbeat) && len(heartbeat.Observations) == 1
		}, 5*time.Second, 10*time.Millisecond, "once it lets go it is asked, and heard, again")
	})

	t.Run("a beat is stamped no later than its kinds were asked", func(t *testing.T) {
		t.Parallel()

		var askedAt atomic.Value

		n := beating(t, registered(t, map[string]func(context.Context) (kind.Report[lampStatus], error){
			"lamp": func(context.Context) (kind.Report[lampStatus], error) {
				askedAt.Store(time.Now())
				time.Sleep(5 * time.Millisecond)

				return kind.Report[lampStatus]{}, nil
			},
		}), time.Second)

		heartbeat, _ := n.beat(t)

		require.Len(t, heartbeat.Observations, 1)
		assert.False(t, heartbeat.At.After(askedAt.Load().(time.Time)), "what it reports was observed no earlier than its stamp")
	})

	t.Run("a node whose stats cannot be read does not beat", func(t *testing.T) {
		t.Parallel()

		var nodeManager runtime.MockNodeManager
		nodeManager.On("Stats", mock.Anything, "node-1").Return(node.Stats{}, errors.New("no cgroup"))

		recorder := &messaging.Recorder{}

		err := NewUseCase(recorder, &nodeManager, nil, time.Second, "node-1", slog.New(slog.DiscardHandler)).Execute(t.Context())

		assert.ErrorContains(t, err, "no cgroup")
		assert.Empty(t, recorder.Messages())
	})

	t.Run("a beat that cannot be sent says so", func(t *testing.T) {
		t.Parallel()

		n := beating(t, nil, time.Second)
		n.recorder.Err = errors.New("nats is away")

		assert.ErrorContains(t, n.heartbeat.Execute(t.Context()), "nats is away")
	})
}

func keys(fields map[string]json.RawMessage) []string {
	var named []string
	for name := range fields {
		named = append(named, name)
	}

	return named
}
