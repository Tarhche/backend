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

// beat beats once, and is the node's own heartbeat it sent, that heartbeat's
// fields as they were written, and the heartbeat it sent of each kind, by
// kind.
func (n *aNode) beat(t *testing.T) (events.Heartbeat, map[string]json.RawMessage, map[string]kind.Heartbeat) {
	t.Helper()

	n.recorder.Reset()
	require.NoError(t, n.heartbeat.Execute(t.Context()))

	messages := n.recorder.Messages()
	require.NotEmpty(t, messages)
	require.Equal(t, events.HeartbeatName, messages[0].Subject, "the node's own goes first")

	var heartbeat events.Heartbeat
	require.NoError(t, json.Unmarshal(messages[0].Payload, &heartbeat))

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(messages[0].Payload, &fields))

	kinds := make(map[string]kind.Heartbeat)

	for _, message := range messages[1:] {
		var held kind.Heartbeat
		require.NoError(t, json.Unmarshal(message.Payload, &held))

		require.Equal(t, kind.HeartbeatName(held.Kind), message.Subject, "each kind's on a subject of its own")
		require.NotContains(t, kinds, held.Kind, "once a beat")

		kinds[held.Kind] = held
	}

	return heartbeat, fields, kinds
}

// refusing is NATS refusing what is said on one subject, and taking the
// rest.
type refusing struct {
	*messaging.Recorder

	subject string
}

func (r refusing) Produce(ctx context.Context, subject string, payload []byte) error {
	if subject == r.subject {
		return errors.New("nats refused it")
	}

	return r.Recorder.Produce(ctx, subject, payload)
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

				heartbeat, fields, held := beating(t, kinds, time.Second).beat(t)

				assert.Equal(t, "node-1", heartbeat.Name)
				assert.Equal(t, node.OrchestratorRole, heartbeat.Role)
				assert.Equal(t, node.Stats{PIDs: 7}, heartbeat.Stats)
				assert.Equal(t, vm.Info{Engine: "memory", CPUs: 8, Memory: 16 << 30, Disk: 100 << 30, Allocated: vm.Resources{CPUs: 2}}, heartbeat.Capacity, "what it offers to VMs, which they are placed by")
				assert.WithinDuration(t, time.Now(), heartbeat.At, time.Minute)

				assert.ElementsMatch(t, []string{"Name", "Role", "Stats", "Capacity", "At"}, keys(fields))
				assert.Empty(t, held, "and nothing of any kind")
			})
		}
	})

	for name, tt := range map[string]struct {
		states map[string]func(context.Context) (kind.Report[lampStatus], error)
		want   map[string]string
		logs   []string
	}{
		"every kind's report is in a heartbeat of its own, with the parents it could not look inside": {
			states: map[string]func(context.Context) (kind.Report[lampStatus], error){
				"lamp":   holding(kind.Report[lampStatus]{Instances: []kind.Observed[lampStatus]{lamp}, Unseen: []string{"room-2"}}),
				"kettle": holding(kind.Report[lampStatus]{}),
			},
			want: map[string]string{
				"lamp":   `{"instances":[{"kind":"lamp","uuid":"lamp-1","owners":[{"kind":"room","uuid":"room-1"}],"status":{"state":"lit","brightness":80}}],"unseen":["room-2"]}`,
				"kettle": `{"instances":[]}`,
			},
		},
		"a kind that could not look sends nothing, and is never reported holding nothing": {
			states: map[string]func(context.Context) (kind.Report[lampStatus], error){
				"lamp": func(context.Context) (kind.Report[lampStatus], error) {
					return kind.Report[lampStatus]{}, errors.New("the fuse box is locked")
				},
				"kettle": holding(kind.Report[lampStatus]{}),
			},
			want: map[string]string{"kettle": `{"instances":[]}`},
			logs: []string{"kind=lamp", "the fuse box is locked"},
		},
		"a kind that takes too long sends nothing, and holds up neither the beat nor the others": {
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
		"and when no kind could say anything, the node's beat goes out all the same": {
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
			heartbeat, _, held := n.beat(t)

			assert.Less(t, time.Since(started), 5*time.Second, "the beat waits for nobody longer than it gives them")
			assert.Equal(t, node.Stats{PIDs: 7}, heartbeat.Stats, "what the node offers is said whatever the kinds said")

			require.Len(t, held, len(tt.want))
			for name, report := range tt.want {
				require.Contains(t, held, name)

				assert.Equal(t, "node-1", held[name].Node)
				assert.True(t, heartbeat.At.Equal(held[name].At), "stamped as its node's beat is")

				encoded, err := json.Marshal(held[name].Report)
				require.NoError(t, err)

				assert.JSONEq(t, report, string(encoded), name)
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

		_, _, held := beating(t, kinds, time.Second).beat(t)

		assert.Empty(t, held)
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

		_, _, held := beating(t, registered(t, states), 10*time.Second).beat(t)

		assert.Len(t, held, many)
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

		_, _, held := n.beat(t)
		assert.Empty(t, held, "it did not answer in time")

		_, _, held = n.beat(t)
		assert.Empty(t, held, "it is still answering the last beat")
		assert.Equal(t, int32(1), asked.Load(), "and is not asked again meanwhile")
		assert.Contains(t, n.logs.String(), "still saying what it holds")

		close(letGo)

		require.Eventually(t, func() bool {
			n.recorder.Reset()

			if err := n.heartbeat.Execute(context.Background()); err != nil {
				return false
			}

			var heartbeat kind.Heartbeat

			return n.recorder.Last(kind.HeartbeatName("lamp"), &heartbeat) && heartbeat.Kind == "lamp"
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

		heartbeat, _, held := n.beat(t)

		require.Contains(t, held, "lamp")
		assert.True(t, held["lamp"].At.Equal(heartbeat.At), "every heartbeat of a beat is stamped alike")
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

	for name, refused := range map[string]string{
		"a node's own heartbeat that cannot be sent keeps no kind's from being": events.HeartbeatName,
		"nor does a kind's keep the node's, or another kind's":                  kind.HeartbeatName("kettle"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			n := beating(t, registered(t, map[string]func(context.Context) (kind.Report[lampStatus], error){
				"lamp":   holding(kind.Report[lampStatus]{}),
				"kettle": holding(kind.Report[lampStatus]{}),
			}), time.Second)
			n.heartbeat.producer = refusing{Recorder: n.recorder, subject: refused}

			assert.ErrorContains(t, n.heartbeat.Execute(t.Context()), "nats refused it", "and the beat says so")

			sent := n.recorder.Subjects()
			assert.NotContains(t, sent, refused)
			assert.Len(t, sent, 2, "the others are sent all the same")
		})
	}
}

// TestUseCase_Hurry holds a node to telling at once what a kind somebody
// watches as it changes holds, a code-runner snippet that ended say, rather
// than at its next beat: in a heartbeat of the kind's own, and only when it
// changed.
func TestUseCase_Hurry(t *testing.T) {
	t.Parallel()

	lit := kind.Report[lampStatus]{Instances: []kind.Observed[lampStatus]{{UUID: "lamp-1", Status: lampStatus{Status: kind.Status{State: lit}, Brightness: 80}}}}
	unlit := kind.Report[lampStatus]{Instances: []kind.Observed[lampStatus]{{UUID: "lamp-1", Status: lampStatus{Status: kind.Status{State: unlit}}}}}

	// what the lamps hold, which a test changes as it goes, and how often the
	// kettles were asked.
	var (
		lock    sync.Mutex
		holds   = lit
		kettled atomic.Int32
	)

	watched := &promptLamps{every: 300 * time.Millisecond, lamps: lamps{state: func(context.Context) (kind.Report[lampStatus], error) {
		lock.Lock()
		defer lock.Unlock()

		return holds, nil
	}}}

	kettles := &lamps{state: func(context.Context) (kind.Report[lampStatus], error) {
		kettled.Add(1)

		return kind.Report[lampStatus]{}, nil
	}}

	kinds := kind.NewRegistry[kind.NodeBinding]()
	require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lampNamed("lamp", kind.OnNode), watched)))
	require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lampNamed("kettle", kind.OnNode), kettles)))

	n := beating(t, kinds, time.Second)

	assert.Equal(t, 300*time.Millisecond, n.heartbeat.Prompt(), "it is hurried as often as its promptest kind asks")

	require.NoError(t, n.heartbeat.Hurry(t.Context()))
	assert.Equal(t, []string{kind.HeartbeatName("lamp")}, n.recorder.Subjects(), "what it holds is told as soon as it is asked, before the node's first beat too: it says nothing of the node")

	n.beat(t)
	n.recorder.Reset()

	asked := kettled.Load()

	require.NoError(t, n.heartbeat.Hurry(t.Context()))
	assert.Empty(t, n.recorder.Messages(), "what was told at the beat is not told again")
	assert.Equal(t, asked, kettled.Load(), "a kind nobody watches as it changes is asked at beats alone")

	lock.Lock()
	holds = unlit
	lock.Unlock()

	require.NoError(t, n.heartbeat.Hurry(t.Context()))

	messages := n.recorder.Messages()
	require.Len(t, messages, 1, "what changed is told at once, and nothing of the node or of the kinds that were not asked")
	require.Equal(t, kind.HeartbeatName("lamp"), messages[0].Subject, "in a heartbeat of its kind's own")

	var heartbeat kind.Heartbeat
	require.NoError(t, json.Unmarshal(messages[0].Payload, &heartbeat))

	assert.Equal(t, "node-1", heartbeat.Node)
	assert.Equal(t, "lamp", heartbeat.Kind)
	assert.WithinDuration(t, time.Now(), heartbeat.At, time.Minute)

	report, err := json.Marshal(heartbeat.Report)
	require.NoError(t, err)
	assert.JSONEq(t, `{"instances":[{"kind":"lamp","uuid":"lamp-1","status":{"state":"unlit"}}]}`, string(report))

	n.recorder.Reset()

	require.NoError(t, n.heartbeat.Hurry(t.Context()))
	assert.Empty(t, n.recorder.Messages(), "and it is told once")

	t.Run("a node that runs no prompt kind is never hurried", func(t *testing.T) {
		t.Parallel()

		assert.Zero(t, beating(t, registered(t, map[string]func(context.Context) (kind.Report[lampStatus], error){"lamp": holding(lit)}), time.Second).heartbeat.Prompt())
	})
}

func keys(fields map[string]json.RawMessage) []string {
	var named []string
	for name := range fields {
		named = append(named, name)
	}

	return named
}
