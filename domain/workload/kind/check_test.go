package kind

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

// changing changes the action of that name in d.
func changing(d *Descriptor, name string, change func(a *Action)) {
	for i := range d.Actions {
		if d.Actions[i].Name == name {
			change(&d.Actions[i])
		}
	}
}

// removing takes the action of that name out of d.
func removing(d *Descriptor, name string) {
	d.Actions = slices.DeleteFunc(d.Actions, func(a Action) bool { return a.Name == name })
}

// ledgerBox is a box whose life is a series of operations the control plane
// carries out, as a snapshot's is: its state is its record's, and no node or
// ingress has anything to do with it.
func ledgerBox() Descriptor {
	d := box()
	d.StateBy = OnControlPlane
	d.Endpoints = false

	removing(&d, "attach")
	removing(&d, "logs")

	for i := range d.Actions {
		d.Actions[i].Runs = OnControlPlane
	}

	return d
}

// unreadable is a codec that reads nothing it writes.
type unreadable struct{}

func (unreadable) Decode([]byte) (any, domain.ValidationErrors, error) {
	return nil, nil, errors.New("it reads nothing")
}

func (unreadable) Type() reflect.Type { return reflect.TypeFor[resizePayload]() }

// misread is a codec that reads what it writes as something else.
type misread struct{ t reflect.Type }

func (misread) Decode([]byte) (any, domain.ValidationErrors, error) {
	return "something else", nil, nil
}
func (m misread) Type() reflect.Type { return m.t }

func TestCheck(t *testing.T) {
	t.Parallel()

	for name, d := range map[string]Descriptor{
		"a kind that keeps every rule":           box(),
		"one whose state is the control plane's": ledgerBox(),
		"and one with no parent": func() Descriptor {
			d := box()
			d.Parent, d.OnParent = "", ParentRules{}

			return d
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, Check(d))
		})
	}

	for name, tt := range map[string]struct {
		breaking func(d *Descriptor)
		want     string
	}{
		"a name that is not a word": {
			breaking: func(d *Descriptor) { d.Name = "Box" },
			want:     `kind "Box": its name is not a lowercase word`,
		},
		"a plural that is not a word": {
			breaking: func(d *Descriptor) { d.Plural = "box es" },
			want:     `its plural "box es" is not a lowercase word`,
		},
		"a state known nowhere": {
			breaking: func(d *Descriptor) { d.StateBy = "" },
			want:     `its state is known neither on a node nor in the control plane ("")`,
		},
		"a machine that does not hold together": {
			breaking: func(d *Descriptor) { d.Machine.Initial = "exploded" },
			want:     `its machine: its initial state "exploded" is not one of its states`,
		},
		"no failed": {
			breaking: func(d *Descriptor) {
				d.Machine.States = slices.DeleteFunc(d.Machine.States, func(s State) bool { return s == Failed })
			},
			want: `its machine has no "failed"`,
		},
		"a failed that is in flight": {
			breaking: func(d *Descriptor) {
				d.Machine.Terminal = slices.DeleteFunc(d.Machine.Terminal, func(s State) bool { return s == Failed })
				d.Machine.InFlight = append(d.Machine.InFlight, Failed)
			},
			want: `"failed" is in flight`,
		},
		"no deleted": {
			breaking: func(d *Descriptor) {
				d.Machine.States = slices.DeleteFunc(d.Machine.States, func(s State) bool { return s == Deleted })
			},
			want: `its machine has no "deleted"`,
		},
		"a deleted that is not terminal": {
			breaking: func(d *Descriptor) {
				d.Machine.Terminal = slices.DeleteFunc(d.Machine.Terminal, func(s State) bool { return s == Deleted })
			},
			want: `"deleted" is not terminal`,
		},
		"a transition out of deleted": {
			breaking: func(d *Descriptor) {
				d.Machine.Transitions = append(d.Machine.Transitions, Transition{From: Deleted, On: OnAction("create"), To: boxStarting})
			},
			want: `a transition on action create leaves "deleted"`,
		},
		"a parent with no rule for its delete": {
			breaking: func(d *Descriptor) { d.OnParent.Delete = "" },
			want:     `it has no rule for what deleting its vm does to it`,
		},
		"nor for its restore": {
			breaking: func(d *Descriptor) { d.OnParent.Restore = "" },
			want:     `it has no rule for what restoring its vm does to it`,
		},
		"a rule a delete cannot follow": {
			breaking: func(d *Descriptor) { d.OnParent.Delete = CascadeReset },
			want:     `"reset" is not what deleting its vm can do to it`,
		},
		"a rule nothing follows": {
			breaking: func(d *Descriptor) { d.OnParent.Restore = "forget" },
			want:     `"forget" is not what restoring its vm can do to it`,
		},
		"rules for a parent it does not have": {
			breaking: func(d *Descriptor) { d.Parent = "" },
			want:     `it has rules for a parent it does not have`,
		},
		"a kind of its own parent": {
			breaking: func(d *Descriptor) { d.Parent = "box" },
			want:     `it is its own parent`,
		},
		"a parent that is not a word": {
			breaking: func(d *Descriptor) { d.Parent = "Docker VM" },
			want:     `its parent "Docker VM" is not a lowercase word`,
		},
		"an action whose name is not a word": {
			breaking: func(d *Descriptor) { changing(d, "stop", func(a *Action) { a.Name = "stop.now" }) },
			want:     `action "stop.now": its name is not a lowercase word`,
		},
		"an action declared twice": {
			breaking: func(d *Descriptor) { d.Actions = append(d.Actions, d.Actions[1]) },
			want:     `action "start" is declared more than once`,
		},
		"an action that runs nowhere": {
			breaking: func(d *Descriptor) { changing(d, "logs", func(a *Action) { a.Runs = "ingress" }) },
			want:     `action "logs" runs neither on a node nor in the control plane ("ingress")`,
		},
		"an action that is asked no way": {
			breaking: func(d *Descriptor) { changing(d, "logs", func(a *Action) { a.Mode = "event" }) },
			want:     `action "logs" is neither a command, a query nor a stream ("event")`,
		},
		"a stream the control plane would serve": {
			breaking: func(d *Descriptor) { changing(d, "attach", func(a *Action) { a.Runs = OnControlPlane }) },
			want:     `action "attach" is a stream, and only a node serves streams`,
		},
		"a query the control plane would answer": {
			breaking: func(d *Descriptor) { changing(d, "logs", func(a *Action) { a.Runs = OnControlPlane }) },
			want:     `action "logs" is a query the control plane has nothing to answer with`,
		},
		"an action allowed in a state it does not have": {
			breaking: func(d *Descriptor) {
				changing(d, "start", func(a *Action) { a.AllowedIn = append(a.AllowedIn, "exploded") })
			},
			want: `action "start" is allowed in "exploded", which is not one of its states`,
		},
		"a query that desires something": {
			breaking: func(d *Descriptor) { changing(d, "logs", func(a *Action) { a.Desires = boxRunning }) },
			want:     `action "logs" desires "running", and only a command asks a resource to be anything`,
		},
		"a command that desires what it does not have": {
			breaking: func(d *Descriptor) { changing(d, "start", func(a *Action) { a.Desires = "exploded" }) },
			want:     `action "start" desires "exploded", which is not one of its states`,
		},
		"an internal action asked under a permission": {
			breaking: func(d *Descriptor) { changing(d, "create", func(a *Action) { a.Permission = "create" }) },
			want:     `action "create" is internal, which nobody asks for, and has a permission`,
		},
		"an internal query": {
			breaking: func(d *Descriptor) {
				changing(d, "logs", func(a *Action) { a.Internal, a.Permission = true, "" })
			},
			want: `action "logs" is internal, and only commands are the workload's own to ask for`,
		},
		"an action asked under no permission": {
			breaking: func(d *Descriptor) { changing(d, "stop", func(a *Action) { a.Permission = "" }) },
			want:     `action "stop" has no permission: the verb of workload.boxes.<verb> it is asked under`,
		},
		"an action asked under what is not a verb": {
			breaking: func(d *Descriptor) { changing(d, "stop", func(a *Action) { a.Permission = "manage it" }) },
			want:     `action "stop" is asked under "manage it", which is not a verb`,
		},
		"an action with no codec": {
			breaking: func(d *Descriptor) { changing(d, "stop", func(a *Action) { a.Payload = nil }) },
			want:     `action "stop" has no codec`,
		},
		"a codec that cannot read its own zero value": {
			breaking: func(d *Descriptor) { changing(d, "resize", func(a *Action) { a.Payload = unreadable{} }) },
			want:     `action "resize": its payload's zero value cannot be read: it reads nothing`,
		},
		"a codec that reads it as something else": {
			breaking: func(d *Descriptor) {
				changing(d, "resize", func(a *Action) { a.Payload = misread{t: reflect.TypeFor[resizePayload]()} })
			},
			want: `action "resize": its payload decodes to a string, not to the kind.resizePayload it says`,
		},
		"a codec of nothing that reads something": {
			breaking: func(d *Descriptor) { changing(d, "stop", func(a *Action) { a.Payload = misread{} }) },
			want:     `action "stop": it is asked with nothing, and nothing decodes to a string`,
		},
		"a codec whose zero value cannot be written": {
			breaking: func(d *Descriptor) {
				changing(d, "resize", func(a *Action) { a.Payload = misread{t: reflect.TypeFor[func()]()} })
			},
			want: `action "resize": its payload's zero value cannot be written`,
		},
		"a transition on an action it does not have": {
			breaking: func(d *Descriptor) {
				d.Machine.Transitions = append(d.Machine.Transitions, Transition{From: boxRunning, On: OnAction("explode"), To: Failed})
			},
			want: `a transition is on action "explode", which it does not have`,
		},
		"a transition on a query": {
			breaking: func(d *Descriptor) {
				d.Machine.Transitions = append(d.Machine.Transitions, Transition{From: boxRunning, On: OnAction("logs"), To: boxStopping})
			},
			want: `a transition is on action "logs", a query, and only a command moves a resource`,
		},
		"a transition from where its action is not allowed": {
			breaking: func(d *Descriptor) {
				d.Machine.Transitions = append(d.Machine.Transitions, Transition{From: boxRunning, On: OnAction("start"), To: boxStarting})
			},
			want: `a transition on action "start" is from "running", where it is not allowed`,
		},
		"no state action": {
			breaking: func(d *Descriptor) { removing(d, "state") },
			want:     `it has no state action`,
		},
		"a state action that is not a query": {
			breaking: func(d *Descriptor) { changing(d, "state", func(a *Action) { a.Mode = ModeCommand }) },
			want:     `its state action is not a query`,
		},
		"a state action that runs where its state is not known": {
			breaking: func(d *Descriptor) { d.StateBy = OnControlPlane },
			want:     `its state action runs on the node, and its state is known on the controlplane`,
		},
		"a state action not allowed in every state": {
			breaking: func(d *Descriptor) {
				changing(d, "state", func(a *Action) { a.AllowedIn = []State{boxRunning} })
			},
			want: `its state action is not allowed in every state`,
		},
		"a state action asked with a payload": {
			breaking: func(d *Descriptor) {
				changing(d, "state", func(a *Action) { a.Payload = Payload[logsPayload]() })
			},
			want: `its state action is asked with a payload`,
		},
		"no delete action": {
			breaking: func(d *Descriptor) {
				removing(d, "delete")
				d.Machine.Transitions = slices.DeleteFunc(d.Machine.Transitions, func(t Transition) bool { return t.On.Action == "delete" })
			},
			want: `it has no delete action`,
		},
		"a delete action that is not a command": {
			breaking: func(d *Descriptor) {
				changing(d, "delete", func(a *Action) { a.Mode, a.Desires = ModeQuery, "" })
			},
			want: `its delete action is not a command`,
		},
		"a delete action that desires otherwise": {
			breaking: func(d *Descriptor) { changing(d, "delete", func(a *Action) { a.Desires = boxStopped }) },
			want:     `its delete action desires "stopped" rather than "deleted"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d := box()
			tt.breaking(&d)

			problems := Check(d)

			assert.Contains(t, messages(problems), tt.want)

			for _, problem := range problems {
				assert.True(t, strings.HasPrefix(problem.Error(), `kind "`+d.Name+`": `), "every problem says whose it is: %v", problem)
			}
		})
	}
}

func TestServices(t *testing.T) {
	t.Parallel()

	attaching := BindNode[boxSpec, boxStatus](box(), &attachingBoxNode{})

	t.Run("a kind with a strategy wherever it runs keeps every rule", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, boxServices(attaching).Check())
	})

	t.Run("and so does one that only the control plane runs", func(t *testing.T) {
		t.Parallel()

		controlPlane := NewRegistry[ControlPlaneBinding]()
		assert.NoError(t, controlPlane.Register(BindControlPlane[boxSpec, boxStatus](ledgerBox(), &boxControlPlane{})))

		assert.Empty(t, Services{ControlPlane: controlPlane}.Check())
	})

	t.Run("its kinds are every kind registered anywhere, once each", func(t *testing.T) {
		t.Parallel()

		services := boxServices(attaching)
		assert.NoError(t, services.Node.Register(BindNode[boxSpec, boxStatus](another("crate", "crates"), &attachingBoxNode{})))

		var names []string
		for _, d := range services.Descriptors() {
			names = append(names, d.Name)
		}

		assert.Equal(t, []string{"box", "crate"}, names)
	})

	for name, tt := range map[string]struct {
		services func() Services
		want     string
	}{
		"a kind the control plane does not keep": {
			services: func() Services {
				services := boxServices(attaching)
				services.ControlPlane = nil

				return services
			},
			want: `kind "box" is not registered in the control plane, which keeps every kind`,
		},
		"a kind described otherwise in one service": {
			services: func() Services {
				d := box()
				changing(&d, "logs", func(a *Action) { a.Permission = "show" })

				nodes := NewRegistry[NodeBinding]()
				assert.NoError(t, nodes.Register(BindNode[boxSpec, boxStatus](d, &attachingBoxNode{})))

				services := boxServices(attaching)
				services.Node = nodes

				return services
			},
			want: `kind "box" is described in the nodes otherwise than it is elsewhere`,
		},
		"an action that runs on a node no node runs": {
			services: func() Services {
				services := boxServices(attaching)
				services.Node = nil

				return services
			},
			want: `kind "box": action "start" runs on a node, and no node strategy is registered for it`,
		},
		"a stream on a node strategy that serves none": {
			services: func() Services { return boxServices(BindNode[boxSpec, boxStatus](box(), &boxNode{})) },
			want:     `kind "box": action "attach" is a stream, and its node strategy serves none`,
		},
		"endpoints on a node strategy that serves no ports": {
			services: func() Services { return boxServices(BindNode[boxSpec, boxStatus](box(), &boxNode{})) },
			want:     `kind "box" has endpoints, and no node strategy serves its ports`,
		},
		"endpoints that no node serves at all": {
			services: func() Services {
				services := boxServices(attaching)
				services.Node = nil

				return services
			},
			want: `kind "box" has endpoints, and no node strategy serves its ports`,
		},
		"a kind reached through the ingress that the ingress cannot find": {
			services: func() Services {
				services := boxServices(attaching)
				services.Ingress = NewRegistry[IngressBinding]()

				return services
			},
			want: `kind "box" is reached through the ingress, and no ingress strategy is registered for it`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, messages(tt.services().Check()), tt.want)
		})
	}
}

// listed are the permissions a box's actions are asked under, as the roles
// page would list them.
func listed() []permission.Permission {
	var all []permission.Permission

	for _, verb := range []string{"manage", "update", "delete", "show", "logs", "attach"} {
		admin, self := box().Permissions(verb)

		all = append(all,
			permission.Permission{Name: verb + " a box", Value: admin},
			permission.Permission{Name: verb + " a self box", Value: self},
		)
	}

	return all
}

func TestCheckPermissions(t *testing.T) {
	t.Parallel()

	t.Run("every action asked under a permission that exists keeps the rule", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, CheckPermissions(box(), listed()))
	})

	t.Run("one asked under a permission that does not exist breaks it, once", func(t *testing.T) {
		t.Parallel()

		without := slices.DeleteFunc(listed(), func(p permission.Permission) bool { return p.Value == "self.workload.boxes.manage" })

		problems := CheckPermissions(box(), without)

		assert.Equal(t, []string{`kind "box": action "start" is asked under "self.workload.boxes.manage", which is not a permission`}, strings.Split(strings.TrimSpace(messages(problems)), "\n"),
			"start and stop share it, and it is said once")
	})

	t.Run("and so does one listed without a name", func(t *testing.T) {
		t.Parallel()

		unnamed := listed()
		for i := range unnamed {
			if unnamed[i].Value == "workload.boxes.logs" {
				unnamed[i].Name = ""
			}
		}

		assert.Contains(t, messages(CheckPermissions(box(), unnamed)), `kind "box": action "logs" is asked under "workload.boxes.logs", which is listed without saying what it grants`)
	})

	t.Run("an internal action is asked under nothing", func(t *testing.T) {
		t.Parallel()

		problems := messages(CheckPermissions(box(), nil))

		assert.Contains(t, problems, `action "start"`, "with nothing listed, every other action breaks it")
		assert.NotContains(t, problems, `action "create"`)
	})
}
