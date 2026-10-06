package kindtest_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kind/kindtest"
)

// A lamp is the least a kind can be: lit and deleted, in the control plane
// alone.

type lampSpec struct {
	Colour string `json:"colour"`
}

const (
	unlit kind.State = "unlit"
	lit   kind.State = "lit"
)

func lamp() kind.Descriptor {
	return kind.Descriptor{
		Name:    "lamp",
		Plural:  "lamps",
		StateBy: kind.OnControlPlane,
		Machine: kind.Machine{
			Initial: unlit,
			States:  []kind.State{unlit, lit, kind.Failed, kind.Deleted},
			Transitions: []kind.Transition{
				{From: unlit, On: kind.OnAction("light"), To: lit},
				{From: kind.Any, On: kind.OnAction("delete"), To: kind.Deleted},
				{From: kind.Any, On: kind.OnObserved(kind.Failed), To: kind.Failed},
			},
			Terminal: []kind.State{kind.Failed, kind.Deleted},
		},
		Actions: []kind.Action{
			{Name: "light", Runs: kind.OnControlPlane, Mode: kind.ModeCommand, AllowedIn: []kind.State{unlit}, Desires: lit, Permission: "manage", Payload: kind.NoPayload},
			{Name: "delete", Runs: kind.OnControlPlane, Mode: kind.ModeCommand, Desires: kind.Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: "state", Runs: kind.OnControlPlane, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// lamps is a lamp's control-plane strategy, which admits and does nothing.
type lamps struct{}

var _ kind.ControlPlane[lampSpec, kind.Status] = lamps{}

func (lamps) Admit(_ context.Context, asked kind.Resource[lampSpec, kind.Status]) (kind.Resource[lampSpec, kind.Status], domain.ValidationErrors, error) {
	return asked, nil, nil
}

func (lamps) Reconcile(context.Context, kind.Resource[lampSpec, kind.Status]) ([]kind.Intent, error) {
	return nil, nil
}

func (lamps) Apply(_ context.Context, r kind.Resource[lampSpec, kind.Status], _ string, _ any) (kind.Resource[lampSpec, kind.Status], domain.ValidationErrors, error) {
	return r, nil, nil
}

// lampServices are the services with a lamp registered where it runs: in
// the control plane.
func lampServices(t *testing.T, d kind.Descriptor) kind.Services {
	t.Helper()

	controlPlane := kind.NewRegistry[kind.ControlPlaneBinding]()
	if err := controlPlane.Register(kind.BindControlPlane[lampSpec, kind.Status](d, lamps{})); err != nil {
		t.Fatal(err)
	}

	return kind.Services{ControlPlane: controlPlane}
}

// permissions are the permissions there are, as the roles page lists them.
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

// lampPermissions are every permission a lamp's actions are asked under.
func lampPermissions() permissions {
	var all permissions

	for _, verb := range []string{"manage", "delete", "show"} {
		admin, self := lamp().Permissions(verb)

		all = append(all,
			permission.Permission{Name: verb + " a lamp", Value: admin},
			permission.Permission{Name: verb + " a self lamp", Value: self},
		)
	}

	return all
}

// recorder is a test that writes down what it is failed for, rather than
// failing, so that a test can say what Conformance fails a test for.
type recorder struct {
	testing.TB

	failures []string
}

func (r *recorder) Helper() {}

func (r *recorder) Error(args ...any) {
	r.failures = append(r.failures, fmt.Sprint(args...))
}

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("a kind with a strategy where it runs, asked under permissions that exist, passes", func(t *testing.T) {
		t.Parallel()

		test := &recorder{TB: t}

		kindtest.Conformance(test, lampServices(t, lamp()), lampPermissions())

		assert.Empty(t, test.failures)
	})

	for name, tt := range map[string]struct {
		services    func(t *testing.T) kind.Services
		permissions permission.Repository
		want        string
	}{
		"services with no kind in them hold nothing to anything": {
			services:    func(*testing.T) kind.Services { return kind.Services{} },
			permissions: lampPermissions(),
			want:        "no kind is registered in any of the services, so none is held to anything",
		},
		"nor do kinds held to no permissions": {
			services: func(t *testing.T) kind.Services { return lampServices(t, lamp()) },
			want:     "no permissions were given to hold the kinds' actions to",
		},
		"an action asked under a permission that does not exist fails": {
			services: func(t *testing.T) kind.Services { return lampServices(t, lamp()) },
			permissions: slices.DeleteFunc(lampPermissions(), func(p permission.Permission) bool {
				return p.Value == "self.workload.lamps.manage"
			}),
			want: `kind "lamp": action "light" is asked under "self.workload.lamps.manage", which is not a permission`,
		},
		"and so does a kind without a strategy where it runs": {
			services: func(t *testing.T) kind.Services {
				d := lamp()
				d.Endpoints = true

				return lampServices(t, d)
			},
			permissions: lampPermissions(),
			want:        `kind "lamp" is reached through the ingress, and no ingress strategy is registered for it`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			test := &recorder{TB: t}

			kindtest.Conformance(test, tt.services(t), tt.permissions)

			assert.Contains(t, test.failures, tt.want)
		})
	}
}
