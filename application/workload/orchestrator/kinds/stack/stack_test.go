package stack_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

// dockerd is one Docker VM's dockerd, as much of it as a stack's node
// strategy asks: whether it answers, and its containers, by label.
type dockerd struct {
	docker.Daemon

	lock       sync.Mutex
	containers []docker.Container
	down       error
	slow       time.Duration
	pinged     int
}

func (d *dockerd) Ping(context.Context) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.pinged++

	return d.down
}

func (d *dockerd) Containers(ctx context.Context, filter docker.ContainerFilter) ([]docker.Container, error) {
	if d.slow > 0 {
		select {
		case <-time.After(d.slow):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if d.down != nil {
		return nil, d.down
	}

	key, value, valued := strings.Cut(filter.Label, "=")

	var listed []docker.Container
	for _, c := range d.containers {
		has, labelled := c.Labels[key]
		if len(filter.Label) > 0 && (!labelled || (valued && has != value)) {
			continue
		}

		listed = append(listed, c)
	}

	return listed, nil
}

func (d *dockerd) set(containers ...docker.Container) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.containers = containers
}

// daemons are the dockerds of a node's Docker VMs.
type daemons map[string]*dockerd

func (d daemons) Daemon(vmUUID string) docker.Daemon {
	if daemon, ok := d[vmUUID]; ok {
		return daemon
	}

	return &dockerd{down: errors.New("no such vm")}
}

// applier is compose as these tests have it: it remembers what it was asked,
// says what it is told to, and changes what dockerd holds as compose would.
type applier struct {
	lock   sync.Mutex
	asked  []string
	output string
	err    error
	then   func(action string)
}

var _ stackKind.Applier = &applier{}

func (a *applier) do(action string, s stackKind.Stack) (string, error) {
	a.lock.Lock()
	a.asked = append(a.asked, action+" "+s.Metadata.Slug)
	then := a.then
	a.lock.Unlock()

	if then != nil && a.err == nil {
		then(action)
	}

	return a.output, a.err
}

func (a *applier) Up(_ context.Context, s stackKind.Stack) (string, error) { return a.do("up", s) }
func (a *applier) Start(_ context.Context, s stackKind.Stack) (string, error) {
	return a.do("start", s)
}

func (a *applier) Stop(_ context.Context, s stackKind.Stack) (string, error) { return a.do("stop", s) }
func (a *applier) Restart(_ context.Context, s stackKind.Stack) (string, error) {
	return a.do("restart", s)
}

func (a *applier) Down(_ context.Context, s stackKind.Stack, removeVolumes bool) (string, error) {
	if removeVolumes {
		return a.do("down --volumes", s)
	}

	return a.do("down", s)
}

func (a *applier) Asked() []string {
	a.lock.Lock()
	defer a.lock.Unlock()

	return slices.Clone(a.asked)
}

// a container of a stack's service, in docker's state.
func container(stackUUID string, service string, n int, state string, status string) docker.Container {
	return docker.Container{
		ID:      fmt.Sprintf("%s-%s-%d", stackUUID, service, n),
		Name:    fmt.Sprintf("shop-%s-%d", service, n),
		State:   state,
		Status:  status,
		Service: service,
		Labels: map[string]string{
			stackKind.LabelStack:       stackUUID,
			stackKind.LabelServices:    "cache,web",
			docker.LabelComposeService: service,
			docker.LabelComposeProject: "shop-abcde",
		},
	}
}

func up(stackUUID string, service string) docker.Container {
	return container(stackUUID, service, 1, "running", "Up 2 minutes")
}

func exitedWith(stackUUID string, service string, code int) docker.Container {
	return container(stackUUID, service, 1, "exited", fmt.Sprintf("Exited (%d) 3 seconds ago", code))
}

// shop is a stack in vm.
func shop(uuid string, vmUUID string) stackKind.Stack {
	return stackKind.Stack{
		Kind: stackKind.Name,
		Metadata: kind.Metadata{
			UUID:   uuid,
			Slug:   "shop-abcde",
			Owners: []kind.Reference{{Kind: stackKind.Parent, UUID: vmUUID}},
			Node:   "node-1",
		},
		Spec: stackKind.Spec{VM: stackKind.VMChoice{UUID: vmUUID}, Compose: "services: {web: {image: nginx}, cache: {image: redis}}"},
	}
}

type fixture struct {
	engine  *memory.Engine
	daemons daemons
	applier *applier
	node    *stack.Node
}

// newFixture is a node with a running Docker VM, vm-1, labelled as one.
func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := &fixture{
		engine:  memory.New(),
		daemons: daemons{"vm-1": &dockerd{}},
		applier: &applier{output: "Container shop-abcde-web-1  Started\n"},
	}

	f.vm(t, "vm-1", true)
	f.node = stack.New(f.engine, f.daemons, f.applier, time.Minute)

	return f
}

// vm makes a VM on the node, a Docker VM labelled as one when docker says.
func (f *fixture) vm(t *testing.T, uuid string, isDocker bool) {
	t.Helper()

	labels := map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelVM: uuid}
	kindOf := vm.KindMachine

	if isDocker {
		labels[docker.LabelVM] = "true"
		kindOf = vm.KindDocker
	}

	_, err := f.engine.Create(t.Context(), vm.Spec{ID: uuid, Kind: kindOf, Labels: labels})
	require.NoError(t, err)
}

func TestNode_Execute(t *testing.T) {
	t.Parallel()

	t.Run("a stack is deployed into its running Docker VM, once its dockerd answers, and is what its containers say", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.applier.then = func(string) { f.daemons["vm-1"].set(up("stack-1", "web"), up("stack-1", "cache")) }

		outcome, err := f.node.Execute(t.Context(), shop("stack-1", "vm-1"), stackKind.ActionCreate, nil)
		require.NoError(t, err)

		assert.Equal(t, []string{"up shop-abcde"}, f.applier.Asked())
		assert.Equal(t, 1, f.daemons["vm-1"].pinged, "its dockerd was waited for first")
		assert.Equal(t, stackKind.Running, outcome.Status.State)
		assert.Equal(t, "Container shop-abcde-web-1  Started\n", outcome.Output)
		assert.Equal(t, outcome.Output, outcome.Status.Output, "what it printed is kept with it")
		require.Len(t, outcome.Status.Services, 2)
		assert.Equal(t, stackKind.ServiceStatus{
			Name:       "web",
			State:      stackKind.ServiceRunning,
			Containers: []stackKind.ContainerStatus{{ID: "stack-1-web-1", Name: "shop-web-1", State: "running"}},
		}, outcome.Status.Services[1])
	})

	t.Run("each command is the applier's own", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			action  string
			payload any
			held    bool
			asked   string
			state   kind.State
		}{
			"applied again": {action: stackKind.ActionApply, held: true, asked: "up shop-abcde", state: stackKind.Running},
			"started":       {action: stackKind.ActionStart, held: true, asked: "start shop-abcde", state: stackKind.Running},
			"started when its vm has none of it, which makes it": {action: stackKind.ActionStart, asked: "up shop-abcde", state: stackKind.Running},
			"restarted":                            {action: stackKind.ActionRestart, held: true, asked: "restart shop-abcde", state: stackKind.Running},
			"restarted when its vm has none of it": {action: stackKind.ActionRestart, asked: "up shop-abcde", state: stackKind.Running},
			"stopped":                              {action: stackKind.ActionStop, held: true, asked: "stop shop-abcde", state: stackKind.Stopped},
			"taken down, its volumes kept":         {action: stackKind.ActionDelete, payload: stackKind.DeletePayload{}, held: true, asked: "down shop-abcde"},
			"taken down with its volumes":          {action: stackKind.ActionDelete, payload: stackKind.DeletePayload{RemoveVolumes: true}, held: true, asked: "down --volumes shop-abcde"},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				f := newFixture(t)
				if tt.held {
					f.daemons["vm-1"].set(exitedWith("stack-1", "web", 0), exitedWith("stack-1", "cache", 0))
				}

				f.applier.then = func(action string) {
					switch action {
					case "stop":
						f.daemons["vm-1"].set(exitedWith("stack-1", "web", 0), exitedWith("stack-1", "cache", 0))
					case "down", "down --volumes":
						f.daemons["vm-1"].set()
					default:
						f.daemons["vm-1"].set(up("stack-1", "web"), up("stack-1", "cache"))
					}
				}

				outcome, err := f.node.Execute(t.Context(), shop("stack-1", "vm-1"), tt.action, tt.payload)
				require.NoError(t, err)

				assert.Equal(t, []string{tt.asked}, f.applier.Asked())
				assert.Equal(t, tt.state, outcome.Status.State)
			})
		}
	})

	t.Run("a stack whose containers cannot be read afterwards is what its command was to make it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.applier.then = func(string) { f.daemons["vm-1"].set() }

		outcome, err := f.node.Execute(t.Context(), shop("stack-1", "vm-1"), stackKind.ActionCreate, nil)
		require.NoError(t, err)
		assert.Equal(t, stackKind.Running, outcome.Status.State)
	})

	t.Run("nothing is applied in a vm that is not running, and the stack fails saying so", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		require.NoError(t, f.engine.Stop(t.Context(), "vm-1"))

		outcome, err := f.node.Execute(t.Context(), shop("stack-1", "vm-1"), stackKind.ActionStart, nil)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
		assert.Equal(t, stackKind.Failed, outcome.Status.State)
		assert.Empty(t, f.applier.Asked())
	})

	t.Run("nor in one whose dockerd does not come up", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.daemons["vm-1"].down = fmt.Errorf("%w: it did not answer", docker.ErrUnavailable)

		_, err := f.node.Execute(t.Context(), shop("stack-1", "vm-1"), stackKind.ActionCreate, nil)
		assert.ErrorIs(t, err, docker.ErrUnavailable)
		assert.Empty(t, f.applier.Asked())
	})

	t.Run("a stack whose vm is not on the node at all has nothing left to take down", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		outcome, err := f.node.Execute(t.Context(), shop("stack-1", "vm-9"), stackKind.ActionDelete, stackKind.DeletePayload{})
		require.NoError(t, err)
		assert.Empty(t, f.applier.Asked())
		assert.Empty(t, outcome.Status.State)

		_, err = f.node.Execute(t.Context(), shop("stack-1", "vm-9"), stackKind.ActionCreate, nil)
		assert.Error(t, err, "and nothing to deploy into")
	})

	t.Run("what the applier printed before it failed is the stack's", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.applier.output = "service \"web\" refers to undefined network backend\n"
		f.applier.err = errors.New("docker compose up exited with 15")

		outcome, err := f.node.Execute(t.Context(), shop("stack-1", "vm-1"), stackKind.ActionCreate, nil)
		assert.ErrorContains(t, err, "exited with 15")
		assert.Equal(t, stackKind.Failed, outcome.Status.State)
		assert.Contains(t, outcome.Status.Output, "undefined network backend")
		assert.Contains(t, outcome.Output, "undefined network backend")
	})

	t.Run("a stack in no vm is not one a command can be carried out on", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		s := shop("stack-1", "")
		s.Metadata.Owners = nil

		_, err := f.node.Execute(t.Context(), s, stackKind.ActionCreate, nil)
		assert.ErrorIs(t, err, kind.ErrInvalidPayload)
	})
}

func TestNode_Query(t *testing.T) {
	t.Parallel()

	_, err := newFixture(t).node.Query(t.Context(), shop("stack-1", "vm-1"), "logs", nil)
	assert.ErrorIs(t, err, kind.ErrUnknownAction)
}

func TestNode_State(t *testing.T) {
	t.Parallel()

	t.Run("the stacks in each running Docker VM are what their containers say", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.vm(t, "vm-2", true)
		f.daemons["vm-2"] = &dockerd{}

		f.daemons["vm-1"].set(
			up("running", "web"), up("running", "cache"),
			up("degraded", "web"), exitedWith("degraded", "cache", 137),
			exitedWith("stopped", "web", 0), exitedWith("stopped", "cache", 0),
			up("half-there", "web"),
			docker.Container{ID: "db", Name: "db", State: "running"},
		)
		f.daemons["vm-2"].set(up("elsewhere", "web"), up("elsewhere", "cache"))

		report, err := f.node.State(t.Context())
		require.NoError(t, err)

		assert.Equal(t, []string{"vm-1", "vm-2"}, report.Read)
		assert.Empty(t, report.Unseen)

		states := map[string]kind.State{}
		for _, instance := range report.Instances {
			states[instance.UUID] = instance.Status.State
		}

		assert.Equal(t, map[string]kind.State{
			"running":    stackKind.Running,
			"degraded":   stackKind.Degraded,
			"stopped":    stackKind.Stopped,
			"half-there": stackKind.Degraded,
			"elsewhere":  stackKind.Running,
		}, states, "a container that is no stack's is none of them")

		halfThere, _ := report.Find("half-there")
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, halfThere.Owners)
		require.Len(t, halfThere.Status.Services, 2)
		assert.Equal(t, stackKind.ServiceMissing, halfThere.Status.Services[0].State, "a service it is to run and has no container of")
		assert.Equal(t, "cache", halfThere.Status.Services[0].Name)
	})

	t.Run("a vm that is not running is not read, and one whose dockerd does not answer is unseen", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.vm(t, "stopped", true)
		f.vm(t, "silent", true)
		f.vm(t, "machine", false)
		f.daemons["silent"] = &dockerd{down: errors.New("Cannot connect to the Docker daemon")}

		require.NoError(t, f.engine.Stop(t.Context(), "stopped"))

		report, err := f.node.State(t.Context())
		require.NoError(t, err)

		assert.Equal(t, []string{"vm-1"}, report.Read)
		assert.Equal(t, []string{"silent"}, report.Unseen)
		assert.True(t, report.Unread("stopped"))
		assert.True(t, report.Unread("machine"), "a vm that is not a Docker VM has no stacks to read")
	})

	t.Run("one too slow to answer before the beat is given up on is unseen, and the rest are read", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.vm(t, "slow", true)
		f.daemons["slow"] = &dockerd{slow: time.Minute}
		f.daemons["vm-1"].set(up("stack-1", "web"), up("stack-1", "cache"))

		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		report, err := f.node.State(ctx)
		require.NoError(t, err)

		assert.Equal(t, []string{"vm-1"}, report.Read)
		assert.Equal(t, []string{"slow"}, report.Unseen)
		assert.Len(t, report.Instances, 1)
	})

	t.Run("a Docker VM made before VMs were labelled is read once a stack's command named it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		_, err := f.engine.Create(t.Context(), vm.Spec{ID: "old", Kind: vm.KindDocker, Labels: map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelVM: "old"}})
		require.NoError(t, err)

		f.daemons["old"] = &dockerd{}

		report, err := f.node.State(t.Context())
		require.NoError(t, err)
		assert.NotContains(t, report.Read, "old")

		_, err = f.node.Execute(t.Context(), shop("stack-1", "old"), stackKind.ActionStart, nil)
		require.NoError(t, err)

		report, err = f.node.State(t.Context())
		require.NoError(t, err)
		assert.Contains(t, report.Read, "old")
	})

	t.Run("a stack a command is being carried out on is said to be on its way, whatever its containers do meanwhile", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		reported := make(chan kind.Report[stackKind.Status], 1)
		f.applier.then = func(string) {
			f.daemons["vm-1"].set(up("stack-1", "web"))

			report, err := f.node.State(t.Context())
			require.NoError(t, err)

			reported <- report

			f.daemons["vm-1"].set(up("stack-1", "web"), up("stack-1", "cache"))
		}

		_, err := f.node.Execute(t.Context(), shop("stack-1", "vm-1"), stackKind.ActionCreate, nil)
		require.NoError(t, err)

		halfway := <-reported
		observed, listed := halfway.Find("stack-1")
		require.True(t, listed)
		assert.Equal(t, stackKind.Deploying, observed.Status.State, "not degraded: it is still being deployed")

		after, err := f.node.State(t.Context())
		require.NoError(t, err)

		observed, _ = after.Find("stack-1")
		assert.Equal(t, stackKind.Running, observed.Status.State)
		assert.Equal(t, "Container shop-abcde-web-1  Started\n", observed.Status.Output, "what its last command printed is said with it")
	})

	t.Run("one that is not on the node yet, while it is deployed, is said to be deploying all the same", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		reported := make(chan kind.Report[stackKind.Status], 1)
		f.applier.then = func(string) {
			report, err := f.node.State(t.Context())
			require.NoError(t, err)

			reported <- report
		}

		_, err := f.node.Execute(t.Context(), shop("stack-1", "vm-1"), stackKind.ActionCreate, nil)
		require.NoError(t, err)

		observed, listed := (<-reported).Find("stack-1")
		require.True(t, listed, "rather than missing from its vm")
		assert.Equal(t, stackKind.Deploying, observed.Status.State)
	})

	t.Run("a node whose engine does not answer sees nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := stack.New(failingEngine{f.engine}, f.daemons, f.applier, time.Minute).State(ctx)
		assert.Error(t, err)
	})
}

func TestStatusOf(t *testing.T) {
	t.Parallel()

	job := func(stackUUID string, code int) docker.Container {
		c := exitedWith(stackUUID, "migrate", code)

		return c
	}

	for name, tt := range map[string]struct {
		containers []docker.Container
		state      kind.State
		services   map[string]stackKind.ServiceState
	}{
		"every service it is to run runs": {
			containers: []docker.Container{up("s", "web"), up("s", "cache")},
			state:      stackKind.Running,
			services:   map[string]stackKind.ServiceState{"web": stackKind.ServiceRunning, "cache": stackKind.ServiceRunning},
		},
		"one that ran once and ended well is not counted against it": {
			containers: []docker.Container{up("s", "web"), up("s", "cache"), job("s", 0)},
			state:      stackKind.Running,
			services:   map[string]stackKind.ServiceState{"web": stackKind.ServiceRunning, "cache": stackKind.ServiceRunning, "migrate": stackKind.ServiceCompleted},
		},
		"nor one that ended badly, which is said as it is": {
			containers: []docker.Container{up("s", "web"), up("s", "cache"), job("s", 1)},
			state:      stackKind.Running,
			services:   map[string]stackKind.ServiceState{"web": stackKind.ServiceRunning, "cache": stackKind.ServiceRunning, "migrate": stackKind.ServiceStopped},
		},
		"one of several containers of a service down is a degraded service, and a degraded stack": {
			containers: []docker.Container{up("s", "web"), container("s", "web", 2, "exited", "Exited (1) 1 second ago"), up("s", "cache")},
			state:      stackKind.Degraded,
			services:   map[string]stackKind.ServiceState{"web": stackKind.ServiceDegraded, "cache": stackKind.ServiceRunning},
		},
		"a service stopped by hand is stopped, whatever it exited with": {
			containers: []docker.Container{exitedWith("s", "web", 0), up("s", "cache")},
			state:      stackKind.Degraded,
			services:   map[string]stackKind.ServiceState{"web": stackKind.ServiceStopped, "cache": stackKind.ServiceRunning},
		},
		"none running is stopped": {
			containers: []docker.Container{exitedWith("s", "web", 0), exitedWith("s", "cache", 137)},
			state:      stackKind.Stopped,
			services:   map[string]stackKind.ServiceState{"web": stackKind.ServiceStopped, "cache": stackKind.ServiceStopped},
		},
		"restarting is not running": {
			containers: []docker.Container{container("s", "web", 1, "restarting", "Restarting (1) 2 seconds ago"), up("s", "cache")},
			state:      stackKind.Degraded,
			services:   map[string]stackKind.ServiceState{"web": stackKind.ServiceStopped, "cache": stackKind.ServiceRunning},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			f.daemons["vm-1"].set(tt.containers...)

			report, err := f.node.State(t.Context())
			require.NoError(t, err)

			observed, listed := report.Find("s")
			require.True(t, listed)

			assert.Equal(t, tt.state, observed.Status.State)

			services := map[string]stackKind.ServiceState{}
			for _, service := range observed.Status.Services {
				services[service.Name] = service.State
			}

			assert.Equal(t, tt.services, services)
		})
	}
}

// failingEngine is an engine that cannot list what it holds.
type failingEngine struct {
	vm.Engine
}

func (failingEngine) List(context.Context) ([]vm.Instance, error) {
	return nil, errors.New("the vmhost is not answering")
}
