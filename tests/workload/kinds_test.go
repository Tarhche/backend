package workload_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orchestratorHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/beatHeart"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	providers "github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/request"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	storageMemory "github.com/khanzadimahdi/testproject/infrastructure/storage/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	infraNode "github.com/khanzadimahdi/testproject/infrastructure/workload/node"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/resources/translation"
)

// A node runs every kind registered on it with the same code, whatever the
// kind: its commands arrive on workloadCommand and what came of them goes
// back on workloadResult, its queries are node requests named after it, and
// what it holds is in every node heartbeat. These tests register a kind of
// their own, a lamp, on a node wired as its serve command wires one, and play
// the control plane's part over a NATS server of the test's own.

type lampSpec struct {
	Watts int `json:"watts"`
}

type lampStatus struct {
	kind.Status

	Brightness int `json:"brightness,omitempty"`
}

const (
	lampUnlit kind.State = "unlit"
	lampLit   kind.State = "lit"
)

func lampKind() kind.Descriptor {
	return kind.Descriptor{
		Name:    "lamp",
		Plural:  "lamps",
		StateBy: kind.OnNode,
		Machine: kind.Machine{
			Initial: lampUnlit,
			States:  []kind.State{lampUnlit, lampLit, kind.Failed, kind.Deleted},
			Transitions: []kind.Transition{
				{From: lampUnlit, On: kind.OnAction("light"), To: lampLit},
				{From: kind.Any, On: kind.OnObserved(kind.Failed), To: kind.Failed},
				{From: kind.Any, On: kind.OnAction("delete"), To: kind.Deleted},
			},
			Terminal: []kind.State{lampUnlit, kind.Failed, kind.Deleted},
		},
		Actions: []kind.Action{
			{Name: "light", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{lampUnlit}, Desires: lampLit, Permission: "manage", Payload: kind.NoPayload},
			{Name: "delete", Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: kind.Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: "state", Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// lampsHeld is a lamp's node strategy: the lamps a node holds, lit by the
// commands it is given.
type lampsHeld struct {
	lock sync.Mutex
	lit  map[string]bool
}

var _ kind.Node[lampSpec, lampStatus] = &lampsHeld{}

func (l *lampsHeld) Execute(_ context.Context, r kind.Resource[lampSpec, lampStatus], action string, _ any) (kind.Outcome[lampStatus], error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	if action == "delete" {
		delete(l.lit, r.Metadata.UUID)

		return kind.Outcome[lampStatus]{Status: lampStatus{Status: kind.Status{State: kind.Deleted}}}, nil
	}

	l.lit[r.Metadata.UUID] = true

	return kind.Outcome[lampStatus]{Status: lampStatus{Status: kind.Status{State: lampLit}, Brightness: r.Spec.Watts * 2}, Output: "click"}, nil
}

func (l *lampsHeld) Query(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (any, error) {
	return nil, nil
}

func (l *lampsHeld) State(context.Context) (kind.Report[lampStatus], error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	report := kind.Report[lampStatus]{}
	for uuid := range l.lit {
		report.Instances = append(report.Instances, kind.Observed[lampStatus]{UUID: uuid, Status: lampStatus{Status: kind.Status{State: lampLit}}})
	}

	return report, nil
}

// aLampResource is a lamp as the control plane records it.
func aLampResource(t *testing.T, uuid string) kind.Raw {
	t.Helper()

	raw, err := kind.Encode(kind.Resource[lampSpec, lampStatus]{
		Kind:     "lamp",
		Metadata: kind.Metadata{UUID: uuid, OwnerUUID: ownerUUID, Node: nodeName},
		Spec:     lampSpec{Watts: 40},
		Status:   lampStatus{Status: kind.Status{State: lampUnlit, Expected: lampLit}},
	})
	require.NoError(t, err)

	return raw
}

// heard is what a test's control plane heard on a subject, as it arrives.
type heard[T any] struct {
	arrived chan T
}

func hearing[T any](t *testing.T, ctx context.Context, messages produceConsumerOf, subject string) *heard[T] {
	t.Helper()

	h := &heard[T]{arrived: make(chan T, 64)}

	require.NoError(t, messages.Consume(ctx, subject, domain.MessageHandlerFunc(func(_ context.Context, data []byte) error {
		var message T
		if err := json.Unmarshal(data, &message); err != nil {
			return nil
		}

		h.arrived <- message

		return nil
	})))

	return h
}

// next is the next message heard that is, waiting for it as long as anything
// here is waited for.
func (h *heard[T]) next(t *testing.T, is func(T) bool) T {
	t.Helper()

	timeout := time.After(settle)

	for {
		select {
		case message := <-h.arrived:
			if is(message) {
				return message
			}
		case <-timeout:
			require.FailNow(t, "nothing was heard in time")
		}
	}
}

// quiet holds that nothing more is heard for a while.
func (h *heard[T]) quiet(t *testing.T, d time.Duration) {
	t.Helper()

	select {
	case message := <-h.arrived:
		assert.Failf(t, "something more was heard", "%+v", message)
	case <-time.After(d):
	}
}

// produceConsumerOf is what a service says and hears over JetStream.
type produceConsumerOf = interface {
	domain.ProduceConsumer
	Wait()
}

func TestKinds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.DiscardHandler)
	english := translator.New(translation.Translations, translation.EN)

	natsURL := natsServer(t)

	// the node, wired as its serve command wires it, with a lamp registered
	// as any kind is.
	nodeConnection := connect(t, natsURL)

	nodeMessages, err := produceConsumer.NewProduceConsumer(nodeConnection, "workload-orchestrator-"+nodeName, logger,
		produceConsumer.WithConcurrency(4),
		produceConsumer.WithProgress(10*time.Second),
	)
	require.NoError(t, err)

	nodeConfigs := configs.NewWorkloadOrchestrator()
	nodeConfigs.Name = nodeName

	engine := memory.New()

	node, err := providers.NewOrchestratorWorkload(providers.OrchestratorDependencies{
		NATS:      nodeConnection,
		Engine:    engine,
		Archives:  storageMemory.New(),
		Producer:  nodeMessages,
		Validator: validator.New(english),
		Configs:   nodeConfigs,
		NodeName:  nodeName,
		Logger:    logger,
	})
	require.NoError(t, err)

	lamps := &lampsHeld{lit: make(map[string]bool)}
	require.NoError(t, node.Kinds.Register(kind.BindNode[lampSpec, lampStatus](lampKind(), lamps)))

	heartbeat := orchestratorHeartbeat.NewUseCase(nodeMessages, infraNode.NewManager(engine), node.Kinds, time.Second, nodeName, logger)

	// the control plane's side of it, listening before anything is said: a
	// JetStream subject keeps a message only for those already listening.
	controlPlaneConnection := connect(t, natsURL)

	controlPlaneMessages, err := produceConsumer.NewProduceConsumer(controlPlaneConnection, "workload-controlplane", logger)
	require.NoError(t, err)

	results := hearing[kind.Result](t, ctx, controlPlaneMessages, kind.ResultName)
	heartbeats := hearing[nodeEvents.Heartbeat](t, ctx, controlPlaneMessages, nodeEvents.HeartbeatName)

	for subject, handler := range node.Subscribers {
		require.NoError(t, nodeMessages.Consume(ctx, subject, handler))
	}

	require.NoError(t, node.Responder.Serve(ctx, noderequest.Subject(nodeName)))

	t.Cleanup(func() {
		cancel()

		controlPlaneMessages.Wait()
		nodeMessages.Wait()
		node.Responder.Wait()
	})

	requester := request.NewRequester(controlPlaneConnection, settle)

	command := func(id string, kindName string, uuid string, action string, node string) {
		t.Helper()

		payload, err := json.Marshal(kind.Command{
			ID:       id,
			Kind:     kindName,
			UUID:     uuid,
			Action:   action,
			Node:     node,
			Resource: aLampResource(t, uuid),
		})
		require.NoError(t, err)

		require.NoError(t, controlPlaneMessages.Produce(ctx, kind.CommandName, payload))
	}

	resultOf := func(id string) func(kind.Result) bool {
		return func(result kind.Result) bool { return result.ID == id }
	}

	t.Run("a command to a lamp on the node is carried out, and what came of it said", func(t *testing.T) {
		command("light-1", "lamp", "lamp-1", "light", nodeName)

		result := results.next(t, resultOf("light-1"))

		assert.True(t, result.OK, result.Reason)
		assert.Equal(t, "lamp-1", result.UUID)
		assert.Equal(t, nodeName, result.Node)
		assert.Equal(t, "click", result.Output)
		assert.JSONEq(t, `{"state":"lit","brightness":80}`, string(result.Status))
		assert.False(t, result.At.IsZero())
	})

	t.Run("one the node cannot carry out is answered as failed, once", func(t *testing.T) {
		command("explode-1", "lamp", "lamp-1", "explode", nodeName)
		command("boil-1", "kettle", "kettle-1", "boil", nodeName)

		exploded := results.next(t, resultOf("explode-1"))
		assert.False(t, exploded.OK)
		assert.Contains(t, exploded.Reason, "unknown action")

		boiled := results.next(t, resultOf("boil-1"))
		assert.False(t, boiled.OK)
		assert.Contains(t, boiled.Reason, "unknown kind")

		// and neither is delivered again, to fail the same way again.
		results.quiet(t, 300*time.Millisecond)
	})

	t.Run("one addressed to another node is left to it", func(t *testing.T) {
		command("light-elsewhere", "lamp", "lamp-2", "light", "workload-orchestrator-02")
		command("light-2", "lamp", "lamp-3", "light", nodeName)

		// the first is never answered: nothing is heard of it before the
		// second's result, or after.
		results.next(t, func(result kind.Result) bool {
			require.NotEqual(t, "light-elsewhere", result.ID, "it is not this node's to answer")

			return result.ID == "light-2"
		})
	})

	t.Run("a lamp's state is asked of its kind, as a node request named after it", func(t *testing.T) {
		asked, err := kind.Query{Kind: "lamp", UUID: "lamp-1", Action: "state", Resource: aLampResource(t, "lamp-1")}.Request()
		require.NoError(t, err)

		reply, err := requester.Request(ctx, nodeName, asked)
		require.NoError(t, err)
		require.NoError(t, reply.Err())

		assert.JSONEq(t, `{"kind":"lamp","uuid":"lamp-1","status":{"state":"lit"}}`, string(reply.Result))
	})

	t.Run("and what is no kind's is not asked of a node at all", func(t *testing.T) {
		reply, err := requester.Request(ctx, nodeName, noderequest.Request{Op: "docker.ping", VMUUID: "no-such-vm"})
		require.NoError(t, err)

		var refused *noderequest.Error
		require.ErrorAs(t, reply.Err(), &refused)
		assert.Equal(t, noderequest.CodeInvalid, refused.Code)
	})

	t.Run("every heartbeat says what the lamps on the node are doing", func(t *testing.T) {
		require.NoError(t, heartbeat.Execute(ctx))

		beat := heartbeats.next(t, func(heartbeat nodeEvents.Heartbeat) bool { return heartbeat.Name == nodeName })

		require.Contains(t, beat.Observations, "lamp")

		var uuids []string
		for _, instance := range beat.Observations["lamp"].Instances {
			uuids = append(uuids, instance.UUID)
			assert.JSONEq(t, `{"state":"lit"}`, string(instance.Status))
		}

		assert.ElementsMatch(t, []string{"lamp-1", "lamp-3"}, uuids)
	})
}
