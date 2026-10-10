package kinds

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// A lamp is the kind these tests route: a lit lamp has a terminal, for its
// owner, and serves its ports under its slug. It is registered through the
// bindings every kind is, so what is tested is a node's routes for a kind as
// the node runs one.

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
			{Name: "attach", Runs: kind.OnNode, Mode: kind.ModeStream, AllowedIn: []kind.State{lit}, Permission: "attach", Payload: kind.NoPayload},
		},
	}
}

// held is a lamp as its node holds it: whose it is, whether it is lit, and
// where each of its ports is published.
type held struct {
	uuid  string
	slug  string
	owner string
	lit   bool
	ports map[port.Port]string
}

// lampNode is what a node knows of the lamps it holds, and the terminals it
// opened in them.
type lampNode struct {
	held []held

	lock      sync.Mutex
	terminals []*echo
}

func (l *lampNode) Execute(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (kind.Outcome[lampStatus], error) {
	return kind.Outcome[lampStatus]{}, nil
}

func (l *lampNode) Query(context.Context, kind.Resource[lampSpec, lampStatus], string, any) (any, error) {
	return nil, nil
}

func (l *lampNode) State(context.Context) (kind.Report[lampStatus], error) {
	return kind.Report[lampStatus]{}, nil
}

// attach opens a terminal in a lit lamp of the owner's, read off the lamp.
func (l *lampNode) attach(uuid string, owner string) (kind.Session, error) {
	for _, lamp := range l.held {
		if lamp.uuid != uuid || lamp.owner != owner {
			continue
		}

		if !lamp.lit {
			return nil, fmt.Errorf("%w: the lamp is not lit", kind.ErrUnreachable)
		}

		terminal := newEcho()

		l.lock.Lock()
		l.terminals = append(l.terminals, terminal)
		l.lock.Unlock()

		return terminal, nil
	}

	return nil, fmt.Errorf("%w: no lamp %q of theirs", domain.ErrNotExists, uuid)
}

// opened are the terminals opened so far.
func (l *lampNode) opened() []*echo {
	l.lock.Lock()
	defer l.lock.Unlock()

	return slices.Clone(l.terminals)
}

// endpoint is where a port of a lit lamp is published.
func (l *lampNode) endpoint(slug string, p port.Port) (kind.Endpoint, error) {
	for _, lamp := range l.held {
		if lamp.slug != slug {
			continue
		}

		if !lamp.lit {
			return kind.Endpoint{}, fmt.Errorf("%w: the lamp is not lit", kind.ErrUnreachable)
		}

		if p == 0 && len(lamp.ports) > 0 {
			p = slices.Min(slices.Collect(maps.Keys(lamp.ports)))
		}

		if address, exposed := lamp.ports[p]; exposed {
			return kind.Endpoint{Port: p, Address: address}, nil
		}

		return kind.Endpoint{}, fmt.Errorf("%w: the lamp does not expose %d", domain.ErrNotExists, p)
	}

	return kind.Endpoint{}, fmt.Errorf("%w: no lamp %q here", domain.ErrNotExists, slug)
}

// lamps serve the lamps' terminals and their ports.
type lamps struct{ *lampNode }

func (l lamps) Attach(_ context.Context, _ string, uuid string, owner string) (kind.Session, error) {
	return l.attach(uuid, owner)
}

func (l lamps) Endpoint(_ context.Context, slug string, p port.Port) (kind.Endpoint, error) {
	return l.endpoint(slug, p)
}

// attachingLamps serve their terminals, and not their ports.
type attachingLamps struct{ *lampNode }

func (l attachingLamps) Attach(_ context.Context, _ string, uuid string, owner string) (kind.Session, error) {
	return l.attach(uuid, owner)
}

// exposingLamps serve their ports, and not their terminals.
type exposingLamps struct{ *lampNode }

func (l exposingLamps) Endpoint(_ context.Context, slug string, p port.Port) (kind.Endpoint, error) {
	return l.endpoint(slug, p)
}

var (
	_ kind.Node[lampSpec, lampStatus] = lamps{}
	_ kind.Attacher                   = lamps{}
	_ kind.Exposer                    = lamps{}
	_ kind.Attacher                   = attachingLamps{}
	_ kind.Exposer                    = exposingLamps{}
)

// running are the kinds a node runs: a lamp, by strategy.
func running(t *testing.T, strategy kind.Node[lampSpec, lampStatus]) *kind.Registry[kind.NodeBinding] {
	t.Helper()

	kinds := kind.NewRegistry[kind.NodeBinding]()
	require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](lamp(), strategy)))

	return kinds
}

// echo is a terminal that says back what it is typed, upper-cased, and
// remembers how big it was made and whether it was closed.
type echo struct {
	stdin  *io.PipeWriter
	stdout *io.PipeReader

	lock    sync.Mutex
	resized []string
	closed  bool
}

var _ kind.Session = &echo{}

func newEcho() *echo {
	typed, stdin := io.Pipe()
	stdout, said := io.Pipe()

	go func() {
		buffer := make([]byte, 64)

		for {
			n, err := typed.Read(buffer)
			if n > 0 {
				if _, err := said.Write(bytes.ToUpper(buffer[:n])); err != nil {
					return
				}
			}

			if err != nil {
				_ = said.Close()

				return
			}
		}
	}()

	return &echo{stdin: stdin, stdout: stdout}
}

func (e *echo) Stdin() io.WriteCloser { return e.stdin }
func (e *echo) Stdout() io.Reader     { return e.stdout }
func (e *echo) Stderr() io.Reader     { return &bytes.Buffer{} }

func (e *echo) Resize(_ context.Context, rows uint, cols uint) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	e.resized = append(e.resized, fmt.Sprintf("%dx%d", rows, cols))

	return nil
}

func (e *echo) Wait(context.Context) (int, error) { return 0, nil }

func (e *echo) Close() error {
	e.lock.Lock()
	e.closed = true
	e.lock.Unlock()

	_ = e.stdin.Close()

	return e.stdout.Close()
}

func (e *echo) sizes() []string {
	e.lock.Lock()
	defer e.lock.Unlock()

	return slices.Clone(e.resized)
}

func (e *echo) isClosed() bool {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.closed
}

// lampControlPlane is a lamp's control-plane strategy, which admits and does
// nothing: the control plane keeps every kind, so a lamp has one to be held
// to the rules every kind keeps.
type lampControlPlane struct{}

var _ kind.ControlPlane[lampSpec, lampStatus] = lampControlPlane{}

func (lampControlPlane) Admit(_ context.Context, asked kind.Resource[lampSpec, lampStatus]) (kind.Resource[lampSpec, lampStatus], domain.ValidationErrors, error) {
	return asked, nil, nil
}

func (lampControlPlane) Reconcile(context.Context, kind.Resource[lampSpec, lampStatus]) ([]kind.Intent, error) {
	return nil, nil
}

func (lampControlPlane) Apply(_ context.Context, r kind.Resource[lampSpec, lampStatus], _ string, _ any) (kind.Resource[lampSpec, lampStatus], domain.ValidationErrors, error) {
	return r, nil, nil
}

// lampIngress finds no lamp: the ingress's side of a lamp, which the
// conformance test asks for since a lamp is reached through the ingress.
type lampIngress struct{}

var _ kind.Ingress = lampIngress{}

func (lampIngress) ByUUID(context.Context, string) (kind.Location, error) {
	return kind.Location{}, domain.ErrNotExists
}

func (lampIngress) BySlug(context.Context, string) (kind.Location, error) {
	return kind.Location{}, domain.ErrNotExists
}

// permissions are the permissions there are, as the roles page lists them:
// a lamp's.
type permissions []permission.Permission

var _ permission.Repository = permissions{}

func (p permissions) GetAll(context.Context) []permission.Permission {
	return p
}

func (p permissions) Get(_ context.Context, values []string) ([]permission.Permission, error) {
	var found []permission.Permission
	for _, each := range p {
		if slices.Contains(values, each.Value) {
			found = append(found, each)
		}
	}

	return found, nil
}

func lampPermissions() permissions {
	var all permissions

	for _, verb := range []string{"manage", "delete", "show", "attach"} {
		admin, self := lamp().Permissions(verb)

		all = append(all,
			permission.Permission{Name: verb + " a lamp", Value: admin},
			permission.Permission{Name: verb + " a self lamp", Value: self},
		)
	}

	return all
}
