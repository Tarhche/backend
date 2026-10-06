package attachResource

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// A lamp is the kind these tests open streams in: a lit lamp has a terminal,
// for its owner. It is registered through the bindings every kind is, so what
// is tested is opening a stream in a kind as a node runs one.

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
			{Name: "light", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{unlit}, Desires: lit, Permission: "manage", Payload: kind.NoPayload},
			{Name: "delete", Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: kind.Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: "state", Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
			{Name: "attach", Runs: kind.OnNode, Mode: kind.ModeStream, AllowedIn: []kind.State{lit}, Permission: "attach", Payload: kind.NoPayload},
		},
	}
}

// held is a lamp as its node holds it: whose it is, and whether it is lit.
type held struct {
	owner string
	lit   bool
}

// lamps is a lamp's node strategy, holding the lamps it does.
type lamps struct {
	held map[string]held

	// opened are the streams it opened, as "action uuid owner".
	opened []string
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

// attachingLamps serve terminals in the lamps they hold too, read off the
// lamp itself: its owner's, and a lit one's only.
type attachingLamps struct {
	lamps
}

var _ kind.Attacher = &attachingLamps{}

func (l *attachingLamps) Attach(_ context.Context, action string, uuid string, owner string) (kind.Session, error) {
	l.opened = append(l.opened, fmt.Sprintf("%s %s %s", action, uuid, owner))

	lamp, found := l.held[uuid]
	if !found || lamp.owner != owner {
		return nil, fmt.Errorf("%w: no lamp %q of theirs", domain.ErrNotExists, uuid)
	}

	if !lamp.lit {
		return nil, fmt.Errorf("%w: the lamp is not lit", kind.ErrUnreachable)
	}

	return &terminal{}, nil
}

// terminal is a session that has said all it will.
type terminal struct {
	output bytes.Buffer
}

var _ kind.Session = &terminal{}

func (t *terminal) Stdin() io.WriteCloser                    { return nopCloser{&t.output} }
func (t *terminal) Stdout() io.Reader                        { return &t.output }
func (t *terminal) Stderr() io.Reader                        { return &bytes.Buffer{} }
func (t *terminal) Resize(context.Context, uint, uint) error { return nil }
func (t *terminal) Wait(context.Context) (int, error)        { return 0, nil }
func (t *terminal) Close() error                             { return nil }

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// running are the kinds a node runs: a lamp, by strategy.
func running(t *testing.T, strategy kind.Node[lampSpec, lampStatus]) *kind.Registry[kind.NodeBinding] {
	t.Helper()

	kinds := kind.NewRegistry[kind.NodeBinding]()
	require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lamp(), strategy)))

	return kinds
}
