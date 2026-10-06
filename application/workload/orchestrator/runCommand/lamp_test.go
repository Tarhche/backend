package runCommand

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// A lamp is the kind these tests command: lit and deleted on the node holding
// it. It is registered through the bindings every kind is, so what is tested
// is the dispatcher with a kind as a node runs one.

type lampSpec struct {
	Watts int `json:"watts"`
}

type lampStatus struct {
	kind.Status

	Brightness int `json:"brightness,omitempty"`
}

// lightPayload is what a lamp is lit with.
type lightPayload struct {
	Brightness int `json:"brightness"`
}

func (p *lightPayload) Validate() domain.ValidationErrors {
	if p.Brightness < 0 || p.Brightness > 100 {
		return domain.ValidationErrors{"brightness": "out_of_range"}
	}

	return nil
}

const (
	unlit kind.State = "unlit"
	lit   kind.State = "lit"
)

func lamp() kind.Descriptor {
	return kind.Descriptor{
		Name:    "lamp",
		Plural:  "lamps",
		StateBy: kind.OnNode,
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
			{Name: "light", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{unlit}, Desires: lit, Permission: "manage", Payload: kind.Payload[lightPayload]()},
			{Name: "delete", Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: kind.Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: "state", Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// executing is what a lamp's strategy does with a command.
type executing func(ctx context.Context, r kind.Resource[lampSpec, lampStatus], action string, payload any) (kind.Outcome[lampStatus], error)

// lamps is a lamp's node strategy: it carries a command out as its test says.
type lamps struct {
	execute executing
}

var _ kind.Node[lampSpec, lampStatus] = &lamps{}

func (l *lamps) Execute(ctx context.Context, r kind.Resource[lampSpec, lampStatus], action string, payload any) (kind.Outcome[lampStatus], error) {
	return l.execute(ctx, r, action, payload)
}

func (l *lamps) Query(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (any, error) {
	return nil, errors.New("a lamp is asked nothing here")
}

func (l *lamps) State(context.Context) (kind.Report[lampStatus], error) {
	return kind.Report[lampStatus]{}, nil
}

// running are the kinds a node runs: a lamp, carried out with execute.
func running(t *testing.T, execute executing) *kind.Registry[kind.NodeBinding] {
	t.Helper()

	kinds := kind.NewRegistry[kind.NodeBinding]()
	require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lamp(), &lamps{execute: execute})))

	return kinds
}

// aCommand asks node-1 for an action on the lamp uuid names, with payload.
func aCommand(t *testing.T, uuid string, action string, payload string) kind.Command {
	t.Helper()

	resource, err := kind.Encode(kind.Resource[lampSpec, lampStatus]{
		Kind:     "lamp",
		Metadata: kind.Metadata{UUID: uuid, Name: "desk", OwnerUUID: "owner-uuid", Node: "node-1"},
		Spec:     lampSpec{Watts: 40},
		Status:   lampStatus{Status: kind.Status{State: unlit, Expected: lit}},
	})
	require.NoError(t, err)

	return kind.Command{
		ID:       "command-" + uuid + "-" + action,
		Kind:     "lamp",
		UUID:     uuid,
		Action:   action,
		Node:     "node-1",
		Attempt:  2,
		Payload:  json.RawMessage(payload),
		Resource: resource,
	}
}
