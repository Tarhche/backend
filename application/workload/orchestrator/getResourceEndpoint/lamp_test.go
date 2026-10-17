package getResourceEndpoint

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// A lamp is the kind these tests reach the ports of: a lit lamp serves its
// ports, under its slug. It is registered through the bindings every kind is,
// so what is tested is finding a port of a kind as a node runs one.

type lampSpec struct {
	Watts int `json:"watts"`
}

type lampStatus struct {
	kind.Status
}

const (
	unlit kind.State = "unlit"
	lit   kind.State = "lit"
)

func lamp() kind.Descriptor {
	return kind.Descriptor{
		Name:      "lamp",
		Plural:    "lamps",
		StateBy:   kind.OnNode,
		Endpoints: true,
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
			{Name: "state", Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// held is a lamp as its node holds it: whether it is lit, and where each of
// its ports is published.
type held struct {
	lit   bool
	ports map[port.Port]string
}

// lamps is a lamp's node strategy, holding the lamps it does by slug.
type lamps struct {
	held map[string]held
}

var _ kind.Node[lampSpec, lampStatus] = &lamps{}

func (l *lamps) Execute(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (kind.Outcome[lampStatus], error) {
	return kind.Outcome[lampStatus]{}, nil
}

func (l *lamps) Query(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (any, error) {
	return nil, nil
}

func (l *lamps) State(context.Context) (kind.Report[lampStatus], error) {
	return kind.Report[lampStatus]{}, nil
}

// exposingLamps serve the ports of the lamps they hold too.
type exposingLamps struct {
	lamps
}

var _ kind.Exposer = &exposingLamps{}

func (l *exposingLamps) Endpoint(_ context.Context, slug string, p port.Port) (kind.Endpoint, error) {
	lamp, found := l.held[slug]
	if !found {
		return kind.Endpoint{}, fmt.Errorf("%w: no lamp %q here", domain.ErrNotExists, slug)
	}

	if !lamp.lit {
		return kind.Endpoint{}, fmt.Errorf("%w: the lamp is not lit", kind.ErrUnreachable)
	}

	if p == 0 && len(lamp.ports) > 0 {
		p = slices.Min(slices.Collect(maps.Keys(lamp.ports)))
	}

	address, exposed := lamp.ports[p]
	if !exposed {
		return kind.Endpoint{}, fmt.Errorf("%w: the lamp does not expose %d", domain.ErrNotExists, p)
	}

	return kind.Endpoint{Port: p, Address: address}, nil
}

// running are the kinds a node runs: a lamp, by strategy.
func running(t *testing.T, strategy kind.Node[lampSpec, lampStatus]) *kind.Registry[kind.NodeBinding] {
	t.Helper()

	kinds := kind.NewRegistry[kind.NodeBinding]()
	require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lamp(), strategy)))

	return kinds
}
