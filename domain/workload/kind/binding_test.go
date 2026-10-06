package kind

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// An engine's terminal is a session a node strategy can hand on as it is.
var _ Session = vm.ExecSession(nil)

// rawBox is a box as it travels.
func rawBox(t *testing.T, change ...func(r *Resource[boxSpec, boxStatus])) Raw {
	t.Helper()

	r := aBox()
	for _, changing := range change {
		changing(&r)
	}

	raw, err := Encode(r)
	require.NoError(t, err)

	return raw
}

func TestControlPlaneBinding(t *testing.T) {
	t.Parallel()

	bound := func() ControlPlaneBinding {
		return BindControlPlane[boxSpec, boxStatus](box(), &boxControlPlane{})
	}

	t.Run("it is bound to its kind's descriptor", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "box", bound().Descriptor().Name)
	})

	t.Run("what is admitted comes back raw, as the kind it was asked as", func(t *testing.T) {
		t.Parallel()

		asked := Raw{
			Metadata: Metadata{Name: "shop", OwnerUUID: "owner-uuid"},
			Spec:     json.RawMessage(`{"image":"nginx:alpine"}`),
		}

		admitted, invalid, err := bound().Admit(context.Background(), asked)
		require.NoError(t, err)
		assert.Empty(t, invalid)

		typed, err := Decode[boxSpec, boxStatus](admitted)
		require.NoError(t, err)

		assert.Equal(t, "box", typed.Kind, "a request need not say what it is: its route does")
		assert.Equal(t, "box-uuid", typed.Metadata.UUID)
		assert.Equal(t, "shop-abcde", typed.Metadata.Slug)
		assert.Equal(t, boxSpec{Image: "nginx:alpine"}, typed.Spec)
		assert.Equal(t, boxPending, typed.Status.State)
	})

	t.Run("what is not admitted says why", func(t *testing.T) {
		t.Parallel()

		admitted, invalid, err := bound().Admit(context.Background(), Raw{Spec: json.RawMessage(`{}`)})

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"image": "required_field"}, invalid)
		assert.Equal(t, Raw{}, admitted)
	})

	t.Run("what cannot be looked at is an error", func(t *testing.T) {
		t.Parallel()

		failure := errors.New("the database is gone")

		_, _, err := BindControlPlane[boxSpec, boxStatus](box(), &boxControlPlane{failure: failure}).Admit(context.Background(), Raw{Spec: json.RawMessage(`{"image":"nginx"}`)})

		assert.ErrorIs(t, err, failure)
	})

	t.Run("a spec that is not the kind's cannot be admitted", func(t *testing.T) {
		t.Parallel()

		_, _, err := bound().Admit(context.Background(), Raw{Spec: json.RawMessage(`{"image": 7}`)})

		assert.ErrorIs(t, err, ErrInvalidPayload)
		assert.ErrorContains(t, err, "the box's spec cannot be read")
	})

	t.Run("what a resource is doing is never the caller's to say", func(t *testing.T) {
		t.Parallel()

		strategy := &boxControlPlane{}

		_, _, err := BindControlPlane[boxSpec, boxStatus](box(), strategy).Admit(context.Background(), Raw{
			Spec:   json.RawMessage(`{"image":"nginx:alpine"}`),
			Status: json.RawMessage(`{"state":"running","uptime":7}`),
		})
		require.NoError(t, err)

		assert.Equal(t, boxStatus{}, strategy.asked.Status)
	})

	t.Run("nor can another kind", func(t *testing.T) {
		t.Parallel()

		_, _, err := bound().Admit(context.Background(), Raw{Kind: "vm", Spec: json.RawMessage(`{"image":"nginx"}`)})

		assert.ErrorIs(t, err, ErrUnknownKind)
	})

	t.Run("a reconcile is the strategy's, of the resource as its own", func(t *testing.T) {
		t.Parallel()

		intents, err := bound().Reconcile(context.Background(), rawBox(t, func(r *Resource[boxSpec, boxStatus]) {
			r.Status.State = boxStopped
		}))
		require.NoError(t, err)

		assert.Equal(t, []Intent{{Action: "start", Reason: "it stopped while it was wanted running"}}, intents)

		intents, err = bound().Reconcile(context.Background(), rawBox(t))
		require.NoError(t, err)
		assert.Empty(t, intents, "a running box wanted running is asked nothing")
	})

	t.Run("a reconcile of another kind is refused", func(t *testing.T) {
		t.Parallel()

		_, err := bound().Reconcile(context.Background(), Raw{Kind: "vm"})

		assert.ErrorIs(t, err, ErrUnknownKind)
	})

	t.Run("and so is an action applied to another kind", func(t *testing.T) {
		t.Parallel()

		_, _, err := bound().Apply(context.Background(), Raw{Kind: "vm"}, "resize", []byte(`{"size": 3}`))

		assert.ErrorIs(t, err, ErrUnknownKind)
	})

	for name, tt := range map[string]struct {
		action  string
		payload string
		size    int
		invalid domain.ValidationErrors
		err     error
	}{
		"an action is applied with its payload, typed": {
			action:  "resize",
			payload: `{"size": 3}`,
			size:    3,
		},
		"a payload that is not valid says why": {
			action:  "resize",
			payload: `{"size": 0}`,
			invalid: domain.ValidationErrors{"size": "required_field"},
		},
		"and so does the strategy, when it refuses": {
			action:  "resize",
			payload: `{"size": 11}`,
			invalid: domain.ValidationErrors{"size": "too_big"},
		},
		"a payload that cannot be read is an error": {
			action:  "resize",
			payload: `{"size": "big"}`,
			err:     ErrInvalidPayload,
		},
		"an action the kind does not have is refused": {
			action: "explode",
			err:    ErrUnknownAction,
		},
		"and so is one that runs on a node": {
			action: "start",
			err:    ErrUnknownAction,
		},
		"and so is a query": {
			action: "state",
			err:    ErrUnknownAction,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			applied, invalid, err := bound().Apply(context.Background(), rawBox(t), tt.action, []byte(tt.payload))

			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.invalid, invalid)

			if len(tt.invalid) > 0 {
				assert.Equal(t, Raw{}, applied)

				return
			}

			typed, err := Decode[boxSpec, boxStatus](applied)
			require.NoError(t, err)
			assert.Equal(t, tt.size, typed.Spec.Size)
			assert.Equal(t, "box", typed.Kind)
		})
	}
}

// aCommand is a box's command, asked of node-1.
func aCommand(t *testing.T, action string, payload string) Command {
	t.Helper()

	return Command{
		ID:       "command-uuid",
		Kind:     "box",
		UUID:     "box-uuid",
		Action:   action,
		Node:     "node-1",
		Attempt:  1,
		Payload:  json.RawMessage(payload),
		Resource: rawBox(t),
	}
}

func TestNodeBinding_Execute(t *testing.T) {
	t.Parallel()

	stopped := boxStatus{Status: Status{State: boxStopped}}

	for name, tt := range map[string]struct {
		strategy *boxNode
		command  func(t *testing.T) Command
		ok       bool
		status   string
		reason   string
		output   string
		executed []string
	}{
		"a command is carried out, and says what it left": {
			strategy: &boxNode{outcome: Outcome[boxStatus]{Status: stopped, Output: "stopped"}},
			command:  func(t *testing.T) Command { return aCommand(t, "stop", "") },
			ok:       true,
			status:   `{"state":"stopped"}`,
			output:   "stopped",
			executed: []string{"stop box-uuid"},
		},
		"a failure says why, and what it left when the strategy said": {
			strategy: &boxNode{outcome: Outcome[boxStatus]{Status: boxStatus{Status: Status{State: Failed}}, Output: "no such image"}, failure: errors.New("pulling nginx:alpine failed")},
			command:  func(t *testing.T) Command { return aCommand(t, "create", "") },
			status:   `{"state":"failed"}`,
			reason:   "pulling nginx:alpine failed",
			output:   "no such image",
			executed: []string{"create box-uuid"},
		},
		"a failure that left nothing it said leaves no status": {
			strategy: &boxNode{failure: errors.New("the vmhost is not answering")},
			command:  func(t *testing.T) Command { return aCommand(t, "start", "") },
			reason:   "the vmhost is not answering",
			executed: []string{"start box-uuid"},
		},
		"a command the kind does not have is not carried out": {
			strategy: &boxNode{},
			command:  func(t *testing.T) Command { return aCommand(t, "explode", "") },
			reason:   "unknown action",
		},
		"nor is a query sent as a command": {
			strategy: &boxNode{},
			command:  func(t *testing.T) Command { return aCommand(t, "logs", "") },
			reason:   "unknown action",
		},
		"nor one that runs in the control plane": {
			strategy: &boxNode{},
			command:  func(t *testing.T) Command { return aCommand(t, "resize", `{"size":3}`) },
			reason:   "unknown action",
		},
		"nor one with a payload it is not asked with": {
			strategy: &boxNode{},
			command:  func(t *testing.T) Command { return aCommand(t, "stop", `{"force":true}`) },
			reason:   "invalid payload",
		},
		"nor one of another kind": {
			strategy: &boxNode{},
			command: func(t *testing.T) Command {
				c := aCommand(t, "stop", "")
				c.Kind = "vm"

				return c
			},
			reason: "unknown kind",
		},
		"nor one carrying a resource that is not the kind's": {
			strategy: &boxNode{},
			command: func(t *testing.T) Command {
				c := aCommand(t, "stop", "")
				c.Resource.Spec = json.RawMessage(`{"image": 7}`)

				return c
			},
			reason: "invalid payload: the box's spec cannot be read",
		},
		"nor one carrying another resource than it names": {
			strategy: &boxNode{},
			command: func(t *testing.T) Command {
				c := aCommand(t, "stop", "")
				c.UUID = "another-box"

				return c
			},
			reason: `it names box "another-box" and carries "box-uuid"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			asked := tt.command(t)

			result := BindNode[boxSpec, boxStatus](box(), tt.strategy).Execute(context.Background(), asked)

			assert.Equal(t, tt.ok, result.OK, "ok")
			assert.Contains(t, result.Reason, tt.reason, "reason")
			assert.Equal(t, tt.output, result.Output, "output")
			assert.Equal(t, tt.executed, tt.strategy.executed, "executed")

			if len(tt.status) > 0 {
				assert.JSONEq(t, tt.status, string(result.Status), "status")
			} else {
				assert.Empty(t, result.Status, "status")
			}

			assert.Equal(t, asked.ID, result.ID, "a result is its command's")
			assert.Equal(t, asked.Kind, result.Kind)
			assert.Equal(t, asked.UUID, result.UUID)
			assert.Equal(t, asked.Action, result.Action)
			assert.Equal(t, asked.Node, result.Node)
			assert.Equal(t, asked.Attempt, result.Attempt)
			assert.True(t, result.At.IsZero(), "it is stamped by whoever sends it")
		})
	}

	t.Run("the strategy is given the payload as its action's own", func(t *testing.T) {
		t.Parallel()

		d := box()
		d.Actions = append(d.Actions, Action{Name: "tail", Runs: OnNode, Mode: ModeCommand, Permission: "logs", Payload: Payload[logsPayload]()})

		strategy := &boxNode{}

		result := BindNode[boxSpec, boxStatus](d, strategy).Execute(context.Background(), aCommand(t, "tail", `{"tail": 5}`))

		require.True(t, result.OK, result.Reason)
		assert.Equal(t, []any{logsPayload{Tail: 5}}, strategy.payloads)
	})

	t.Run("and none, for an action asked with nothing", func(t *testing.T) {
		t.Parallel()

		strategy := &boxNode{}

		BindNode[boxSpec, boxStatus](box(), strategy).Execute(context.Background(), aCommand(t, "stop", ""))

		assert.Equal(t, []any{nil}, strategy.payloads)
	})

	t.Run("output is its end, as much of it as a result carries", func(t *testing.T) {
		t.Parallel()

		strategy := &boxNode{outcome: Outcome[boxStatus]{Output: strings.Repeat("a", MaxOutput) + "the end"}}

		result := BindNode[boxSpec, boxStatus](box(), strategy).Execute(context.Background(), aCommand(t, "stop", ""))

		assert.Len(t, result.Output, MaxOutput)
		assert.True(t, strings.HasSuffix(result.Output, "the end"))
	})
}

// aQuery is a box's query, as it arrives on node-1.
func aQuery(t *testing.T, action string, payload string) Query {
	t.Helper()

	return Query{Kind: "box", UUID: "box-uuid", Action: action, Payload: json.RawMessage(payload), Resource: rawBox(t)}
}

func TestNodeBinding_Query(t *testing.T) {
	t.Parallel()

	running := Observed[boxStatus]{
		UUID:   "box-uuid",
		Owners: []Reference{{Kind: "vm", UUID: "vm-uuid"}},
		Status: boxStatus{Status: Status{State: boxRunning}, Uptime: 42},
	}

	for name, tt := range map[string]struct {
		strategy *boxNode
		query    func(t *testing.T) Query
		answer   string
		err      error
		says     string
	}{
		"a query is the strategy's answer, as json": {
			strategy: &boxNode{},
			query:    func(t *testing.T) Query { return aQuery(t, "logs", `{"tail": 20}`) },
			answer:   `["logs of box-uuid: the last 20 lines"]`,
		},
		"a payload that is not valid is refused with why": {
			strategy: &boxNode{},
			query:    func(t *testing.T) Query { return aQuery(t, "logs", `{"tail": 5000}`) },
			err:      ErrInvalidPayload,
			says:     "tail: too_many",
		},
		"what the strategy could not answer is its error": {
			strategy: &boxNode{failure: domain.ErrNotExists},
			query:    func(t *testing.T) Query { return aQuery(t, "logs", ``) },
			err:      domain.ErrNotExists,
		},
		"a command is not a query": {
			strategy: &boxNode{},
			query:    func(t *testing.T) Query { return aQuery(t, "stop", ``) },
			err:      ErrUnknownAction,
		},
		"a query of another kind is refused": {
			strategy: &boxNode{},
			query: func(t *testing.T) Query {
				q := aQuery(t, "logs", ``)
				q.Kind = "vm"

				return q
			},
			err: ErrUnknownKind,
		},
		"a resource's state is read off what the node holds": {
			strategy: &boxNode{report: Report[boxStatus]{Instances: []Observed[boxStatus]{running}}},
			query:    func(t *testing.T) Query { return aQuery(t, "state", ``) },
			answer:   `{"kind":"box","uuid":"box-uuid","owners":[{"kind":"vm","uuid":"vm-uuid"}],"status":{"state":"running","uptime":42}}`,
		},
		"one the node does not hold, inside a parent it read, is observed missing": {
			strategy: &boxNode{report: Report[boxStatus]{Read: []string{"vm-uuid"}}},
			query:    func(t *testing.T) Query { return aQuery(t, "state", ``) },
			answer:   `{"kind":"box","uuid":"box-uuid","status":{"state":"missing"}}`,
		},
		"one inside a parent the node could not see into is not known": {
			strategy: &boxNode{report: Report[boxStatus]{Unseen: []string{"vm-uuid"}}},
			query:    func(t *testing.T) Query { return aQuery(t, "state", ``) },
			err:      ErrUnseen,
		},
		"nor is one inside a parent the node did not look into, which is not running there": {
			strategy: &boxNode{report: Report[boxStatus]{Read: []string{"vm-2"}}},
			query:    func(t *testing.T) Query { return aQuery(t, "state", ``) },
			err:      ErrUnreachable,
		},
		"and neither is any, when the node could see nothing": {
			strategy: &boxNode{blind: errors.New("the vmhost is not answering")},
			query:    func(t *testing.T) Query { return aQuery(t, "state", ``) },
			says:     "the vmhost is not answering",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			answer, err := BindNode[boxSpec, boxStatus](box(), tt.strategy).Query(context.Background(), tt.query(t))

			if tt.err != nil || len(tt.says) > 0 {
				if tt.err != nil {
					assert.ErrorIs(t, err, tt.err)
				}

				if len(tt.says) > 0 {
					assert.ErrorContains(t, err, tt.says)
				}

				assert.Nil(t, answer)

				return
			}

			require.NoError(t, err)
			assert.JSONEq(t, tt.answer, string(answer))
		})
	}
}

func TestNodeBinding_State(t *testing.T) {
	t.Parallel()

	t.Run("what the node holds is reported raw, every instance said to be of the kind", func(t *testing.T) {
		t.Parallel()

		strategy := &boxNode{report: Report[boxStatus]{
			Instances: []Observed[boxStatus]{
				{UUID: "box-1", Owners: []Reference{{Kind: "vm", UUID: "vm-1"}}, Status: boxStatus{Status: Status{State: boxRunning}, Uptime: 7}},
				{Owners: []Reference{{Kind: "vm", UUID: "vm-1"}}, Status: boxStatus{Status: Status{State: boxStopped}}},
			},
			Read:   []string{"vm-1"},
			Unseen: []string{"vm-2"},
		}}

		report, err := BindNode[boxSpec, boxStatus](box(), strategy).State(context.Background())
		require.NoError(t, err)

		require.Len(t, report.Instances, 2)
		assert.Equal(t, "box", report.Instances[0].Kind)
		assert.Equal(t, "box-1", report.Instances[0].UUID)
		assert.Equal(t, []Reference{{Kind: "vm", UUID: "vm-1"}}, report.Instances[0].Owners)
		assert.JSONEq(t, `{"state":"running","uptime":7}`, string(report.Instances[0].Status))
		assert.Equal(t, "box", report.Instances[1].Kind)
		assert.Empty(t, report.Instances[1].UUID, "one nobody keeps a record of is reported all the same")
		assert.Equal(t, []string{"vm-1"}, report.Read)
		assert.Equal(t, []string{"vm-2"}, report.Unseen)

		strategy.report.Read[0] = "changed"
		strategy.report.Unseen[0] = "changed"
		assert.Equal(t, []string{"vm-1"}, report.Read, "what is reported is not the strategy's to change")
		assert.Equal(t, []string{"vm-2"}, report.Unseen, "what is reported is not the strategy's to change")
	})

	t.Run("a node that could see nothing says so", func(t *testing.T) {
		t.Parallel()

		blind := errors.New("the vmhost is not answering")

		_, err := BindNode[boxSpec, boxStatus](box(), &boxNode{blind: blind}).State(context.Background())

		assert.ErrorIs(t, err, blind)
	})
}

func TestNodeBinding_Attach(t *testing.T) {
	t.Parallel()

	t.Run("a strategy that serves streams opens one for the owner", func(t *testing.T) {
		t.Parallel()

		session := &terminal{}
		bound := BindNode[boxSpec, boxStatus](box(), &attachingBoxNode{session: session})

		assert.True(t, bound.Attaches())

		opened, err := bound.Attach(context.Background(), "attach", "box-uuid", "owner-uuid")
		require.NoError(t, err)
		assert.Same(t, session, opened)

		_, err = bound.Attach(context.Background(), "attach", "box-uuid", "somebody-else")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a box that is not theirs is not there")
	})

	t.Run("only a stream is opened", func(t *testing.T) {
		t.Parallel()

		_, err := BindNode[boxSpec, boxStatus](box(), &attachingBoxNode{}).Attach(context.Background(), "logs", "box-uuid", "owner-uuid")

		assert.ErrorIs(t, err, ErrUnknownAction)
	})

	t.Run("a strategy that serves none opens none", func(t *testing.T) {
		t.Parallel()

		bound := BindNode[boxSpec, boxStatus](box(), &boxNode{})

		assert.False(t, bound.Attaches())

		_, err := bound.Attach(context.Background(), "attach", "box-uuid", "owner-uuid")
		assert.ErrorIs(t, err, ErrUnknownAction)
	})
}

func TestNodeBinding_Endpoint(t *testing.T) {
	t.Parallel()

	t.Run("a strategy that serves ports says where one is", func(t *testing.T) {
		t.Parallel()

		bound := BindNode[boxSpec, boxStatus](box(), &attachingBoxNode{})

		assert.True(t, bound.Exposes())

		endpoint, err := bound.Endpoint(context.Background(), "shop-abcde", 0)
		require.NoError(t, err)
		assert.Equal(t, Endpoint{Port: 8080, Address: "vmhost-01:20000"}, endpoint, "no port named is the lowest it exposes")

		_, err = bound.Endpoint(context.Background(), "shop-abcde", 22)
		assert.ErrorIs(t, err, domain.ErrNotExists, "a port it does not expose is not there")
	})

	t.Run("a strategy that serves none has nothing there", func(t *testing.T) {
		t.Parallel()

		bound := BindNode[boxSpec, boxStatus](box(), &boxNode{})

		assert.False(t, bound.Exposes())

		_, err := bound.Endpoint(context.Background(), "shop-abcde", 8080)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestIngressBinding(t *testing.T) {
	t.Parallel()

	bound := BindIngress(box(), boxIngress{})

	assert.Equal(t, "box", bound.Descriptor().Name)

	located, err := bound.BySlug(context.Background(), "shop-abcde")
	require.NoError(t, err)
	assert.Equal(t, Location{UUID: "box-uuid", Node: "node-1", Ports: []port.Port{8080}}, located)

	_, err = bound.ByUUID(context.Background(), "another-box")
	assert.ErrorIs(t, err, domain.ErrNotExists)
}
