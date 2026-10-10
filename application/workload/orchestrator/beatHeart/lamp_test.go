package beatHeart

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Lamps are the kinds these tests' node holds: lit and deleted on the node,
// each kind of lamp a kind of its own, so a node can run several. They are
// registered through the bindings every kind is, so what is tested is the
// heartbeat with kinds as a node runs them.

type lampSpec struct {
	Watts int `json:"watts"`
}

type lampStatus struct {
	kind.Status

	Brightness int `json:"brightness,omitempty"`
}

const (
	unlit kind.State = "unlit"
	lit   kind.State = "lit"
)

// lampNamed is a kind of lamp of that name, whose state is known where
// statedBy says.
func lampNamed(name string, statedBy kind.Executor) kind.Descriptor {
	return kind.Descriptor{
		Name:    name,
		Plural:  name + "s",
		StateBy: statedBy,
		Machine: kind.Machine{
			Initial: unlit,
			States:  []kind.State{unlit, lit, kind.Failed, kind.Deleted},
			Transitions: []kind.Transition{
				{From: unlit, On: kind.OnAction("light"), To: lit},
				{From: kind.Any, On: kind.OnObserved(kind.Failed), To: kind.Failed},
				{From: kind.Any, On: kind.OnAction("delete"), To: kind.Deleted},
			},
			Terminal: []kind.State{unlit, kind.Failed, kind.Deleted},
		},
		Actions: []kind.Action{
			{Name: "light", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{unlit}, Desires: lit, Permission: "manage", Payload: kind.NoPayload},
			{Name: "delete", Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: kind.Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: "state", Runs: statedBy, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// lamps is a kind of lamp's node strategy, holding what its state says.
type lamps struct {
	state func(ctx context.Context) (kind.Report[lampStatus], error)
}

var _ kind.Node[lampSpec, lampStatus] = &lamps{}

func (l *lamps) Execute(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (kind.Outcome[lampStatus], error) {
	return kind.Outcome[lampStatus]{}, nil
}

func (l *lamps) Query(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (any, error) {
	return nil, nil
}

func (l *lamps) State(ctx context.Context) (kind.Report[lampStatus], error) {
	return l.state(ctx)
}

// holding is a strategy that holds what report says.
func holding(report kind.Report[lampStatus]) func(context.Context) (kind.Report[lampStatus], error) {
	return func(context.Context) (kind.Report[lampStatus], error) {
		return report, nil
	}
}

// registered are kinds of lamps, each stated on the node and holding what its
// state says.
func registered(t *testing.T, states map[string]func(context.Context) (kind.Report[lampStatus], error)) *kind.Registry[kind.NodeBinding] {
	t.Helper()

	kinds := kind.NewRegistry[kind.NodeBinding]()

	for name, state := range states {
		require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lampNamed(name, kind.OnNode), &lamps{state: state})))
	}

	return kinds
}

// promptLamps are lamps somebody watches as they change: their node asks
// what they hold between beats too, every every.
type promptLamps struct {
	lamps

	every time.Duration
}

func (l *promptLamps) Prompt() time.Duration {
	return l.every
}
