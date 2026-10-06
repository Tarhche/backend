package answerQuery

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// A lamp is the kind these tests ask about: lit, read and deleted on the
// node holding it, inside the room it is in. It is registered through the
// bindings every kind is, so what is tested is the routing with a kind as a
// node runs one.

type lampSpec struct {
	Watts int `json:"watts"`
}

type lampStatus struct {
	kind.Status

	Brightness int `json:"brightness,omitempty"`
}

// readingsPayload is what a lamp's readings are asked with.
type readingsPayload struct {
	Last int `json:"last"`
}

func (p *readingsPayload) Validate() domain.ValidationErrors {
	if p.Last < 0 || p.Last > 100 {
		return domain.ValidationErrors{"last": "out_of_range"}
	}

	return nil
}

const (
	unlit kind.State = "unlit"
	lit   kind.State = "lit"
)

func lamp() kind.Descriptor {
	return kind.Descriptor{
		Name:     "lamp",
		Plural:   "lamps",
		StateBy:  kind.OnNode,
		Parent:   "room",
		OnParent: kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset},
		Machine: kind.Machine{
			Initial: unlit,
			States:  []kind.State{unlit, lit, kind.Failed, kind.Missing, kind.Deleted},
			Transitions: []kind.Transition{
				{From: unlit, On: kind.OnAction("light"), To: lit},
				{From: lit, On: kind.OnObserved(kind.Missing), To: kind.Missing},
				{From: kind.Any, On: kind.OnObserved(kind.Failed), To: kind.Failed},
				{From: kind.Any, On: kind.OnAction("delete"), To: kind.Deleted},
			},
			Terminal: []kind.State{unlit, kind.Failed, kind.Deleted},
		},
		Actions: []kind.Action{
			{Name: "light", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{unlit}, Desires: lit, Permission: "manage", Payload: kind.NoPayload},
			{Name: "delete", Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: kind.Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: "state", Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
			{Name: "readings", Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.Payload[readingsPayload]()},
		},
	}
}

// lamps is a lamp's node strategy: it answers its readings, or fails with
// failure, and holds what report says.
type lamps struct {
	failure error

	report kind.Report[lampStatus]
	blind  error

	// asked is every query it answered.
	asked []string
}

var _ kind.Node[lampSpec, lampStatus] = &lamps{}

func (l *lamps) Execute(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (kind.Outcome[lampStatus], error) {
	return kind.Outcome[lampStatus]{}, nil
}

func (l *lamps) Query(_ context.Context, r kind.Resource[lampSpec, lampStatus], action string, payload any) (any, error) {
	l.asked = append(l.asked, fmt.Sprintf("%s %s %d watts %v", action, r.Metadata.UUID, r.Spec.Watts, payload))

	if l.failure != nil {
		return nil, l.failure
	}

	return []int{3, 2, 1}[:payload.(readingsPayload).Last], nil
}

func (l *lamps) State(context.Context) (kind.Report[lampStatus], error) {
	return l.report, l.blind
}

// running are the kinds a node runs: a lamp, with strategy.
func running(t *testing.T, strategy *lamps) *kind.Registry[kind.NodeBinding] {
	t.Helper()

	kinds := kind.NewRegistry[kind.NodeBinding]()
	require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lamp(), strategy)))

	return kinds
}

// aLamp is the lamp lamp-1, in room-1, as the control plane recorded it.
func aLamp(t *testing.T) kind.Raw {
	t.Helper()

	raw, err := kind.Encode(kind.Resource[lampSpec, lampStatus]{
		Kind: "lamp",
		Metadata: kind.Metadata{
			UUID:      "lamp-1",
			OwnerUUID: "owner-uuid",
			Owners:    []kind.Reference{{Kind: "room", UUID: "room-1"}},
			Node:      "node-1",
		},
		Spec:   lampSpec{Watts: 40},
		Status: lampStatus{Status: kind.Status{State: lit}},
	})
	require.NoError(t, err)

	return raw
}
