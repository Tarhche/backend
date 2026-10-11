package stack_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/ownership"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements/requirementstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/stack"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
)

// shop is a stack's file: a web server on its front and back networks, after
// an api on the back one, after a database there with a volume and a bind,
// and a worker, which names no network.
const shop = `
services:
  web:
    image: nginx:1.27
    ports: ["8080:80"]
    depends_on: [api]
    networks: [front, back]
  api:
    image: shop/api:1
    environment:
      DATABASE: postgres://db/shop
    depends_on: [db]
    networks: [back]
  db:
    image: postgres:17
    volumes: ["data:/var/lib/postgresql/data", "/srv/initdb:/docker-entrypoint-initdb.d:ro"]
    networks: [back]
    restart: unless-stopped
    pids_limit: 200
    privileged: true
    user: postgres
    runtime: runc
    resources:
      limits: {cpus: 0.5, memory: 512m}
  worker:
    image: shop/worker:1
    command: work --queue 'jobs and more'
volumes:
  data:
networks:
  front:
  back:
`

// silence is how long what lives in a VM may go unheard while its node beats
// before it is taken to be gone from it, which a stack its VM's restore reset
// waits for, from the first look into the restored disk, before it is found
// not to be there.
const silence = 30 * time.Second

// stackGraph is requirementstest's workload, with the stack kind registered
// as the control plane registers it, its strategy planning stacks' graphs, and
// the kinds a stack's graph is made of, its own among them.
type stackGraph struct {
	*requirementstest.Workload

	strategy *stack.Stacks
	stacks   *stack.Graph
	kinds    graph.Kinds
	planner  *graph.Planner
}

// newStackGraph is a stack's graph's kinds over the VMs options say, its node
// carrying out at once whatever it is sent, and the stack kind's strategy,
// giving slugs nothing holds.
func newStackGraph(t *testing.T, options ...vmtest.Option) *stackGraph {
	t.Helper()

	w := requirementstest.New(options...)

	g := &stackGraph{Workload: w, stacks: stack.NewGraph(w.Requirements, silence), kinds: maps.Clone(w.Kinds)}
	g.kinds[stackKind.Name] = g.stacks
	g.planner = graph.NewPlanner(g.kinds, graph.DefaultPolicy())
	g.strategy = g.strategyHolding()

	require.NoError(t, w.Registry.Register(kind.BindControlPlane[stackKind.Spec, stackKind.Status](stackKind.Descriptor(), g.strategy)))

	return g
}

// strategyHolding is the stack kind's strategy over g, to which the slugs
// held says are held, and no other.
func (g *stackGraph) strategyHolding(held ...slugs.Taken) *stack.Stacks {
	return stack.New(stack.Dependencies{Chooser: g.Chooser, Slugs: held, Planner: g.planner})
}

// aStack is a stack of owner's in the Docker VM vmUUID names, under the slug
// shop, of file, deploying, as changes say otherwise.
func aStack(uuid string, vmUUID string, file string, changes ...func(s *stackKind.Stack)) kind.Raw {
	s := stackKind.Stack{
		Kind: stackKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "shop",
			Slug:      "shop",
			OwnerUUID: "owner",
			Refs:      []kind.Reference{{Kind: vmKind.Name, UUID: vmUUID}},
			CreatedAt: time.Now().Add(-time.Hour),
		},
		Spec: stackKind.Spec{VM: stackKind.VMChoice{UUID: vmUUID}, Compose: file},
		Status: stackKind.Status{
			Status: kind.Status{State: stackKind.Deploying, Expected: stackKind.Running},
		},
	}

	for _, change := range changes {
		change(&s)
	}

	raw, err := kind.Encode(s)
	if err != nil {
		panic(err)
	}

	return raw
}

// wantOf is the stack r is the record of, as its graph needs it.
func wantOf(t *testing.T, r kind.Raw) stackKind.Want {
	t.Helper()

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](r)
	require.NoError(t, err)

	w, err := stack.WantOf(s)
	require.NoError(t, err)

	return w
}

// storedStack is the stack kept under uuid, and whether one is.
func storedStack(t *testing.T, g *stackGraph, uuid string) (stackKind.Stack, resource.Record, bool) {
	t.Helper()

	r, kept := g.Kept(stackKind.Name, uuid)
	if !kept {
		return stackKind.Stack{}, resource.Record{}, false
	}

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](r.Raw)
	require.NoError(t, err)

	return s, r, true
}

func TestGraph_Root(t *testing.T) {
	t.Parallel()

	stacks := newStackGraph(t).stacks

	t.Run("a stack is known by its record's uuid, its owner's, under its slug, in the vm its spec names, made of its file", func(t *testing.T) {
		t.Parallel()

		r := aStack("s-1", "vm-1", shop, func(s *stackKind.Stack) {
			s.Spec.VM = stackKind.VMChoice{UUID: "vm-1", Name: "builds", Ports: []port.Port{}, Slug: "builds-abcde"}
		})

		required, err := stacks.Root(r)
		require.NoError(t, err)

		file, invalid := stackKind.Parse(shop)
		require.Empty(t, invalid)

		assert.Equal(t, graph.Requirement{Kind: stackKind.Name, Name: "s-1", Spec: stackKind.Want{
			UUID:      "s-1",
			OwnerUUID: "owner",
			Slug:      "shop",
			VM:        stackKind.VMChoice{UUID: "vm-1", Name: "builds", Ports: []port.Port{}, Slug: "builds-abcde"},
			File:      file,
		}}, required)
	})

	for name, tt := range map[string]struct {
		record kind.Raw
		err    error
		says   string
	}{
		"what is not a stack": {
			record: blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running),
			err:    kind.ErrUnknownKind,
		},
		"one in no vm named by its uuid": {
			record: aStack("s-1", "", shop),
			err:    kind.ErrInvalidPayload,
		},
		"one with no slug": {
			record: aStack("s-1", "vm-1", shop, func(s *stackKind.Stack) { s.Metadata.Slug = "" }),
			err:    kind.ErrInvalidPayload,
		},
		"one whose file cannot be read, kept before its file was the platform's": {
			record: aStack("s-1", "vm-1", "services:\n  web:\n    build: .\n"),
			err:    kind.ErrInvalidPayload,
			says:   "compose.services.web.build: unsupported_key",
		},
		"one whose file says an alias, kept before the platform refused them, which is never followed": {
			record: aStack("s-1", "vm-1", "services:\n  web: &web\n    image: nginx\n  api: *web\n"),
			err:    kind.ErrInvalidPayload,
			says:   "compose.services.api: unsupported_alias",
		},
	} {
		t.Run("not "+name, func(t *testing.T) {
			t.Parallel()

			_, err := stacks.Root(tt.record)

			require.ErrorIs(t, err, tt.err)
			assert.Contains(t, err.Error(), tt.says)
		})
	}
}

func TestGraph_Requires(t *testing.T) {
	t.Parallel()

	stacks := newStackGraph(t).stacks

	t.Run("its vm, the networks it makes, its volumes, and a container for each service, made after those it depends on", func(t *testing.T) {
		t.Parallel()

		w := wantOf(t, aStack("s-1", "vm-1", shop))

		required, err := stacks.Requires(w)
		require.NoError(t, err)

		ids := make([]graph.NodeID, len(required))
		for i, r := range required {
			ids[i] = r.ID()
		}

		assert.Equal(t, []graph.NodeID{
			"vm/vm-1",
			"network/shop_back", "network/shop_default", "network/shop_front",
			"volume/shop_data",
			"container/shop_api", "container/shop_db", "container/shop_web", "container/shop_worker",
		}, ids, "every network it declares, and the default one the worker joins by naming none")

		after := map[graph.NodeID][]graph.NodeID{}
		for _, r := range required {
			if len(r.After) > 0 {
				after[r.ID()] = r.After
			}
		}

		assert.Equal(t, map[graph.NodeID][]graph.NodeID{
			"container/shop_api": {"container/shop_db"},
			"container/shop_web": {"container/shop_api"},
		}, after, "each after what its service depends on")

		assert.Equal(t, vmKind.Want{OwnerUUID: "owner", VM: w.VM}, required[0].Spec)
		assert.Equal(t, networkKind.Want{OwnerUUID: "owner", VM: w.VM, Spec: networkKind.Spec{Name: "shop_back"}}, required[1].Spec, "as a container that joins it needs it, so that the two are one")
		assert.Equal(t, volumeKind.Want{OwnerUUID: "owner", VM: w.VM, Spec: volumeKind.Spec{Name: "shop_data"}}, required[4].Spec)
	})

	t.Run("a service's container is asked for as its service says, named under the stack's slug, and owned and labelled as its", func(t *testing.T) {
		t.Parallel()

		w := wantOf(t, aStack("s-1", "vm-1", shop))

		required, err := stacks.Requires(&w)
		require.NoError(t, err, "a want is handed over as one, or as a pointer to one")

		wants := map[string]containerKind.Want{}
		for _, r := range required {
			if r.Kind == containerKind.Name {
				wants[r.Name] = r.Spec.(containerKind.Want)
			}
		}

		owned := []kind.Reference{{Kind: stackKind.Name, UUID: "s-1"}}

		assert.Equal(t, containerKind.Want{
			OwnerUUID: "owner",
			Labels:    map[string]string{stackKind.LabelStack: "s-1", stackKind.LabelService: "db"},
			Owners:    owned,
			VM:        w.VM,
			Spec: containerKind.Spec{
				Name:  "shop_db",
				Image: "postgres:17",
				Mounts: []containerKind.Mount{
					{Type: containerKind.MountVolume, Source: "shop_data", Target: "/var/lib/postgresql/data"},
					{Type: containerKind.MountBind, Source: "/srv/initdb", Target: "/docker-entrypoint-initdb.d", ReadOnly: true},
				},
				Networks:      []string{"shop_back"},
				Aliases:       map[string][]string{"shop_back": {"db"}},
				RestartPolicy: containerKind.RestartUnlessStopped,
				CPUs:          0.5,
				Memory:        512 << 20,
				PidsLimit:     200,
				Privileged:    true,
				User:          "postgres",
				Runtime:       "runc",
			},
		}, wants["shop_db"], "a volume named under the slug, a bind as it is")

		web := wants["shop_web"].Spec
		assert.Equal(t, []string{"shop_back", "shop_front"}, web.Networks)
		assert.Equal(t, map[string][]string{"shop_back": {"web"}, "shop_front": {"web"}}, web.Aliases, "reached by its service's name on each of its networks")
		assert.Equal(t, []containerKind.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}, web.Ports)
		assert.Equal(t, containerKind.RestartNo, web.RestartPolicy)

		api := wants["shop_api"].Spec
		assert.Equal(t, []string{"DATABASE=postgres://db/shop"}, api.Env)

		worker := wants["shop_worker"].Spec
		assert.Equal(t, []string{"work", "--queue", "jobs and more"}, worker.Command)
		assert.Equal(t, []string{"shop_default"}, worker.Networks)
		assert.Equal(t, map[string][]string{"shop_default": {"worker"}}, worker.Aliases)

		for name, want := range wants {
			assert.Equal(t, owned, want.Owners, "%s is the stack's from its admission on", name)
			assert.Empty(t, want.UUID, "%s is known by its name", name)
			assert.Empty(t, want.Spec.External, "%s has nothing that is not its own", name)
		}
	})

	t.Run("no default network when every service names one", func(t *testing.T) {
		t.Parallel()

		w := wantOf(t, aStack("s-1", "vm-1", "services:\n  web:\n    image: nginx\n    networks: [front]\nnetworks:\n  front:\n"))

		required, err := stacks.Requires(w)
		require.NoError(t, err)

		ids := make([]graph.NodeID, len(required))
		for i, r := range required {
			ids[i] = r.ID()
		}

		assert.Equal(t, []graph.NodeID{"vm/vm-1", "network/shop_front", "container/shop_web"}, ids)
	})

	for name, spec := range map[string]any{
		"what is not a stack's want": containerKind.Want{VM: stackKind.VMChoice{UUID: "vm-1"}},
		"one with no uuid":           stackKind.Want{Slug: "shop", VM: stackKind.VMChoice{UUID: "vm-1"}},
		"one in no vm":               stackKind.Want{UUID: "s-1", Slug: "shop"},
		"one under no slug":          stackKind.Want{UUID: "s-1", VM: stackKind.VMChoice{UUID: "vm-1"}},
		"nothing":                    nil,
	} {
		t.Run("not "+name, func(t *testing.T) {
			t.Parallel()

			_, err := stacks.Requires(spec)

			assert.ErrorIs(t, err, kind.ErrInvalidPayload)
		})
	}
}

func TestGraph_Requires_stopped(t *testing.T) {
	t.Parallel()

	stacks := newStackGraph(t).stacks

	// containers are what each container a stack requires is wanted as, by
	// its name.
	containers := func(t *testing.T, w stackKind.Want) map[string]containerKind.Want {
		t.Helper()

		required, err := stacks.Requires(w)
		require.NoError(t, err)

		wanted := map[string]containerKind.Want{}
		for _, r := range required {
			if c, ok := r.Spec.(containerKind.Want); ok {
				wanted[c.Spec.Name] = c
			}
		}

		require.Len(t, wanted, 4)

		return wanted
	}

	t.Run("a stack expected stopped wants each of its containers made stopped", func(t *testing.T) {
		t.Parallel()

		w := wantOf(t, aStack("s-1", "vm-1", shop, func(s *stackKind.Stack) {
			s.Status.State = stackKind.Stopped
			s.Status.Expected = stackKind.Stopped
		}))
		require.True(t, w.Stopped)

		for name, c := range containers(t, w) {
			assert.True(t, c.Stopped, "%s", name)
		}
	})

	t.Run("and one expected running, running", func(t *testing.T) {
		t.Parallel()

		w := wantOf(t, aStack("s-1", "vm-1", shop))
		require.False(t, w.Stopped)

		for name, c := range containers(t, w) {
			assert.False(t, c.Stopped, "%s", name)
		}
	})
}

func TestGraph_plan(t *testing.T) {
	t.Parallel()

	g := newStackGraph(t)

	t.Run("its vm first, then its networks and its volumes, then its services a level at a time, as they depend on one another, and the stack last", func(t *testing.T) {
		t.Parallel()

		plan, err := graph.NewPlanner(g.kinds, graph.DefaultPolicy()).Plan(wantOf(t, aStack("s-1", "vm-1", shop)).Requirement())
		require.NoError(t, err)

		assert.Equal(t, [][]graph.NodeID{
			{"vm/vm-1"},
			{"network/shop_back", "network/shop_default", "network/shop_front", "volume/shop_data"},
			{"container/shop_db", "container/shop_worker"},
			{"container/shop_api"},
			{"container/shop_web"},
			{"stack/s-1"},
		}, plan.Levels())
	})

	t.Run("one vm, and one network for every service on it, each needed by each that needs it", func(t *testing.T) {
		t.Parallel()

		plan, err := graph.NewPlanner(g.kinds, graph.DefaultPolicy()).Plan(wantOf(t, aStack("s-1", "vm-1", shop)).Requirement())
		require.NoError(t, err)

		assert.Len(t, plan.Nodes(), 10)

		needing := func(id graph.NodeID) []graph.NodeID {
			var dependents []graph.NodeID
			for _, e := range plan.Dependents(id) {
				dependents = append(dependents, e.To)
			}

			return dependents
		}

		assert.Equal(t, []graph.NodeID{"container/shop_api", "container/shop_db", "container/shop_web", "stack/s-1"}, needing("network/shop_back"))
		assert.Equal(t, []graph.NodeID{"container/shop_worker", "stack/s-1"}, needing("network/shop_default"))
		assert.Equal(t, []graph.NodeID{"container/shop_db", "stack/s-1"}, needing("volume/shop_data"))
		assert.Len(t, needing("vm/vm-1"), 9, "the vm of everything in it, made once")
	})

	t.Run("its edges typed by the policy: its containers and networks owned, its volumes and vm used", func(t *testing.T) {
		t.Parallel()

		plan, err := graph.NewPlanner(g.kinds, graph.DefaultPolicy()).Plan(wantOf(t, aStack("s-1", "vm-1", shop)).Requirement())
		require.NoError(t, err)

		for edge, want := range map[[2]graph.NodeID]graph.EdgeType{
			{"container/shop_web", "stack/s-1"}:          graph.Owned,
			{"network/shop_front", "stack/s-1"}:          graph.Owned,
			{"volume/shop_data", "stack/s-1"}:            graph.Loose,
			{"vm/vm-1", "stack/s-1"}:                     graph.Loose,
			{"network/shop_back", "container/shop_db"}:   graph.Owned,
			{"volume/shop_data", "container/shop_db"}:    graph.Loose,
			{"vm/vm-1", "container/shop_db"}:             graph.Loose,
			{"container/shop_db", "container/shop_api"}:  graph.Order,
			{"container/shop_api", "container/shop_web"}: graph.Order,
		} {
			e, there := plan.Edge(edge[0], edge[1])
			require.True(t, there, "%s → %s", edge[0], edge[1])
			assert.Equal(t, want, e.Type, "%s → %s", edge[0], edge[1])
		}

		assert.Equal(t, []graph.NodeID{
			"container/shop_api", "container/shop_db", "container/shop_web", "container/shop_worker",
			"network/shop_back", "network/shop_default", "network/shop_front",
		}, plan.Owned("stack/s-1"), "what removing it takes with it: its containers and its networks")
	})

	t.Run("with a stack's volumes owned by it, it takes them with it, and no kind says otherwise", func(t *testing.T) {
		t.Parallel()

		owning := graph.DefaultPolicy().With(graph.DependencyPolicy{ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Owned}})

		plan, err := graph.NewPlanner(g.kinds, owning).Plan(wantOf(t, aStack("s-1", "vm-1", shop)).Requirement())
		require.NoError(t, err)

		assert.Contains(t, plan.Owned("stack/s-1"), graph.NodeID("volume/shop_data"))

		e, _ := plan.Edge("volume/shop_data", "container/shop_db")
		assert.Equal(t, graph.Loose, e.Type, "and its containers still only use them")
	})

	t.Run("services that depend on one another in a ring are a cycle of their containers, which no order makes", func(t *testing.T) {
		t.Parallel()

		ring := "services:\n  web:\n    image: nginx\n    depends_on: [api]\n  api:\n    image: api\n    depends_on: [db]\n  db:\n    image: postgres\n    depends_on: [web]\n"

		_, err := graph.NewPlanner(g.kinds, graph.DefaultPolicy()).Plan(wantOf(t, aStack("s-1", "vm-1", ring)).Requirement())

		var cycle *graph.Cycle
		require.ErrorAs(t, err, &cycle)
		assert.ElementsMatch(t, []graph.NodeID{"container/shop_api", "container/shop_db", "container/shop_web"}, cycle.Nodes)
	})
}

// running is what a container's node says of one that runs.
func running(c *containerKind.Container) {
	c.Status.Docker.State = "running"
	c.Status.Docker.Status = "Up 3 hours"
}

// exited is what a container's node says of one that exited with code, as
// its restart policy says it is.
func exited(code int, state kind.State) func(c *containerKind.Container) {
	return func(c *containerKind.Container) {
		c.Status.Status.State = state
		c.Status.Docker.State = "exited"
		c.Status.Docker.Status = "Exited (" + strconv.Itoa(code) + ") 3 seconds ago"
	}
}

// serviceContainer is the container of a stack's service, as its graph made
// it, running, as changes say otherwise.
func serviceContainer(uuid string, stackUUID string, service string, changes ...func(c *containerKind.Container)) kind.Raw {
	return blockstest.AContainer(uuid, "vm-1", containerKind.Running, containerKind.Running, append([]func(c *containerKind.Container){func(c *containerKind.Container) {
		c.Metadata.Name = "shop_" + service
		c.Metadata.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: stackUUID}}
		c.Metadata.Labels = map[string]string{stackKind.LabelStack: stackUUID, stackKind.LabelService: service}
		c.Spec.Name = "shop_" + service
		c.Status.Docker.Name = "shop_" + service
	}, running}, changes...)...)
}

// twoServices is a stack's file of a web server and a one-off that runs to
// its end.
const twoServices = "services:\n  web:\n    image: nginx\n  migrate:\n    image: shop/migrate:1\n"

// deps are the records a stack's graph made of the services of the stack
// twoServices is the file of, by their nodes.
var deps = map[graph.NodeID]graph.Ref{
	"vm/vm-1":                {Kind: vmKind.Name, UUID: "vm-1"},
	"network/shop_default":   {Kind: networkKind.Name, UUID: "n-1"},
	"container/shop_web":     {Kind: containerKind.Name, UUID: "c-web"},
	"container/shop_migrate": {Kind: containerKind.Name, UUID: "c-migrate"},
}

func TestGraph_Create(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		web      []func(c *containerKind.Container)
		migrate  []func(c *containerKind.Container)
		state    kind.State
		services map[string]stackKind.ServiceState
	}{
		"each of its services runs: it runs": {
			state:    stackKind.Running,
			services: map[string]stackKind.ServiceState{"web": stackKind.ServiceRunning, "migrate": stackKind.ServiceRunning},
		},
		"a one-off that ran to its end and exited with nothing is as it was to be": {
			migrate:  []func(c *containerKind.Container){exited(0, containerKind.Completed)},
			state:    stackKind.Running,
			services: map[string]stackKind.ServiceState{"web": stackKind.ServiceRunning, "migrate": stackKind.ServiceCompleted},
		},
		"but one that exited with anything else stopped": {
			migrate:  []func(c *containerKind.Container){exited(1, containerKind.Completed)},
			state:    stackKind.Degraded,
			services: map[string]stackKind.ServiceState{"web": stackKind.ServiceRunning, "migrate": stackKind.ServiceStopped},
		},
		"and so did one asked to stop, however it exited": {
			migrate: []func(c *containerKind.Container){exited(0, containerKind.Completed), func(c *containerKind.Container) {
				c.Status.Expected = containerKind.Stopped
			}},
			state:    stackKind.Degraded,
			services: map[string]stackKind.ServiceState{"web": stackKind.ServiceRunning, "migrate": stackKind.ServiceStopped},
		},
		"none of its services runs: it is stopped": {
			web:      []func(c *containerKind.Container){exited(0, containerKind.Stopped)},
			migrate:  []func(c *containerKind.Container){exited(0, containerKind.Completed)},
			state:    stackKind.Stopped,
			services: map[string]stackKind.ServiceState{"web": stackKind.ServiceStopped, "migrate": stackKind.ServiceCompleted},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
			g.Keep(aStack("s-1", "vm-1", twoServices))
			g.Keep(serviceContainer("c-web", "s-1", "web", tt.web...))
			g.Keep(serviceContainer("c-migrate", "s-1", "migrate", tt.migrate...))

			ref, err := g.stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), deps)
			require.NoError(t, err)

			assert.Equal(t, graph.Ref{Kind: stackKind.Name, UUID: "s-1"}, ref)

			s, _, kept := storedStack(t, g, "s-1")
			require.True(t, kept)

			assert.Equal(t, tt.state, s.Status.State)

			states := map[string]stackKind.ServiceState{}
			for _, service := range s.Status.Services {
				states[service.Name] = service.State
			}

			assert.Equal(t, tt.services, states)
		})
	}

	t.Run("its record says each service's container by its uuid, as the stack's foreign keys, and what docker says it is doing", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		g.Keep(aStack("s-1", "vm-1", twoServices, func(s *stackKind.Stack) { s.Status.Reason = "its vm is being made" }))
		g.Keep(serviceContainer("c-web", "s-1", "web"))
		g.Keep(serviceContainer("c-migrate", "s-1", "migrate", exited(0, containerKind.Completed)))

		_, err := g.stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), deps)
		require.NoError(t, err)

		s, _, _ := storedStack(t, g, "s-1")

		assert.Equal(t, []stackKind.ServiceStatus{
			{Name: "migrate", State: stackKind.ServiceCompleted, Containers: []stackKind.ContainerStatus{{ID: "c-migrate", Name: "shop_migrate", State: "exited"}}},
			{Name: "web", State: stackKind.ServiceRunning, Containers: []stackKind.ContainerStatus{{ID: "c-web", Name: "shop_web", State: "running"}}},
		}, s.Status.Services)
		assert.Empty(t, s.Status.Reason, "what it waited on is let go of")
	})

	t.Run("it is written only when what its containers make it changed", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		g.Keep(aStack("s-1", "vm-1", twoServices))
		g.Keep(serviceContainer("c-web", "s-1", "web"))
		g.Keep(serviceContainer("c-migrate", "s-1", "migrate"))

		_, err := g.stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), deps)
		require.NoError(t, err)

		_, first, _ := storedStack(t, g, "s-1")

		_, err = g.stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), deps)
		require.NoError(t, err)

		_, again, _ := storedStack(t, g, "s-1")
		assert.Equal(t, first.Version, again.Version, "nothing was written the second time")
	})

	t.Run("one whose record is gone is gone, and is never admitted again", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		_, err := g.stacks.Create(ctx, wantOf(t, aStack("s-gone", "vm-1", twoServices)), deps)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is gone")

		_, _, kept := storedStack(t, g, "s-gone")
		assert.False(t, kept)
	})

	t.Run("one on its way out waits, and is not written", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		g.Keep(aStack("s-1", "vm-1", twoServices, func(s *stackKind.Stack) {
			s.Status.Expected = stackKind.Deleted
			s.Metadata.DeletionRequestedAt = time.Now()
		}))

		_, err := g.stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), deps)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
	})

	t.Run("a service its graph made nothing of is no stack made", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		g.Keep(aStack("s-1", "vm-1", twoServices))
		g.Keep(serviceContainer("c-web", "s-1", "web"))

		_, err := g.stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), map[graph.NodeID]graph.Ref{"container/shop_web": {Kind: containerKind.Name, UUID: "c-web"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "migrate")

		s, _, _ := storedStack(t, g, "s-1")
		assert.Equal(t, stackKind.Deploying, s.Status.State, "as it was")
	})
}

// interloping keeps records as the repository it wraps does, but does what a
// test says the first time the record of a kind under a uuid is read: what a
// person asks of a stack while its graph reads what it needs, say.
type interloping struct {
	resource.Repository

	kindName string
	uuid     string
	do       func()
	once     sync.Once
}

func (i *interloping) GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error) {
	if kindName == i.kindName && uuid == i.uuid {
		i.once.Do(i.do)
	}

	return i.Repository.GetOne(ctx, kindName, uuid)
}

// interloped is the stack kind as a graph needs it, over g's records, but
// for doing what do says the first time the record of a kind under a uuid is
// read.
func interloped(g *stackGraph, kindName string, uuid string, do func()) *stack.Graph {
	dependencies := g.Requirements.Dependencies
	dependencies.Resources = &interloping{Repository: g.Resources, kindName: kindName, uuid: uuid, do: do}

	return stack.NewGraph(requirements.New(dependencies), silence)
}

func TestGraph_Create_askedMeanwhile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// atRest is a stack at rest, as it is asked to be.
	atRest := func(state kind.State, expected kind.State) func(s *stackKind.Stack) {
		return func(s *stackKind.Stack) {
			s.Status.State = state
			s.Status.Expected = expected
		}
	}

	t.Run("a stop asked once the stack was read, made again once a container of it exited, is carried out, not said over as what its containers made it", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		carrying(g)

		g.Keep(aStack("s-1", "vm-1", twoServices, atRest(stackKind.Running, stackKind.Running)))
		g.Keep(serviceContainer("c-web", "s-1", "web"))
		g.Keep(serviceContainer("c-migrate", "s-1", "migrate", exited(1, containerKind.Completed)))

		// stopped as its graph reads its containers, after it read the stack.
		stacks := interloped(g, containerKind.Name, "c-web", func() { askStack(t, g, stackKind.ActionStop) })

		_, err := stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), deps)
		require.NoError(t, err)

		s, _, _ := storedStack(t, g, "s-1")
		assert.Equal(t, stackKind.Stopped, s.Status.State, "stopped, as it was asked, and not degraded, as it was when it was read")
		assert.Equal(t, stackKind.Stopped, s.Status.Expected)
		assert.Equal(t, []string{"stop container/shop_web"}, commands(g, containerKind.Name))

		for _, service := range []string{"web", "migrate"} {
			c := blockRecord(t, g, containerKind.Name, "shop_"+service)
			common, err := c.Common()
			require.NoError(t, err)
			assert.Equal(t, containerKind.Stopped, common.Expected, "%s is held stopped", service)
		}
	})

	t.Run("and so is a start of one that waited, its containers started in order", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		carrying(g)

		g.Keep(aStack("s-1", "vm-1", twoServices, atRest(stackKind.Waiting, stackKind.Running)))
		g.Keep(serviceContainer("c-web", "s-1", "web", exited(0, containerKind.Stopped)))
		g.Keep(serviceContainer("c-migrate", "s-1", "migrate", exited(0, containerKind.Stopped)))

		stacks := interloped(g, containerKind.Name, "c-web", func() { askStack(t, g, stackKind.ActionStart) })

		_, err := stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), deps)
		require.NoError(t, err)

		s, _, _ := storedStack(t, g, "s-1")
		assert.Equal(t, stackKind.Running, s.Status.State, "started, and not stopped, as it was when it was read")
		assert.ElementsMatch(t, []string{"start container/shop_migrate", "start container/shop_web"}, commands(g, containerKind.Name))
	})
}

func TestGraph_Exists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := stackKind.Want{UUID: "s-1", OwnerUUID: "owner", Slug: "shop", VM: stackKind.VMChoice{UUID: "vm-1"}}

	made := func(t *testing.T) *stackGraph {
		t.Helper()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		g.Keep(aStack("s-1", "vm-1", twoServices))
		g.Keep(serviceContainer("c-web", "s-1", "web"))
		g.Keep(serviceContainer("c-migrate", "s-1", "migrate"))

		_, err := g.stacks.Create(ctx, wantOf(t, aStack("s-1", "vm-1", twoServices)), deps)
		require.NoError(t, err)

		return g
	}

	w.File, _ = stackKind.Parse(twoServices)

	t.Run("one saying what its containers make it is made", func(t *testing.T) {
		t.Parallel()

		ref, there, err := made(t).stacks.Exists(ctx, w)
		require.NoError(t, err)

		assert.True(t, there)
		assert.Equal(t, graph.Ref{Kind: stackKind.Name, UUID: "s-1"}, ref)
	})

	t.Run("one whose container stopped since is its record all the same, and not made", func(t *testing.T) {
		t.Parallel()

		g := made(t)
		require.NoError(t, g.Memory.Delete(ctx, containerKind.Name, "c-migrate"))
		g.Keep(serviceContainer("c-migrate", "s-1", "migrate", exited(137, containerKind.Stopped)))

		ref, there, err := g.stacks.Exists(ctx, w)
		require.NoError(t, err)

		assert.False(t, there)
		assert.Equal(t, graph.Ref{Kind: stackKind.Name, UUID: "s-1"}, ref)
	})

	t.Run("and so is one whose container was made again under another uuid", func(t *testing.T) {
		t.Parallel()

		g := made(t)
		require.NoError(t, g.Memory.Delete(ctx, containerKind.Name, "c-migrate"))
		g.Keep(serviceContainer("c-migrate-2", "s-1", "migrate"))

		_, there, err := g.stacks.Exists(ctx, w)
		require.NoError(t, err)

		assert.False(t, there)
	})

	t.Run("and one not made yet", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		g.Keep(aStack("s-1", "vm-1", twoServices))

		ref, there, err := g.stacks.Exists(ctx, w)
		require.NoError(t, err)

		assert.False(t, there)
		assert.Equal(t, graph.Ref{Kind: stackKind.Name, UUID: "s-1"}, ref)
	})

	t.Run("and one asked to go", func(t *testing.T) {
		t.Parallel()

		g := made(t)
		_, r, _ := storedStack(t, g, "s-1")
		r.Metadata.DeletionRequestedAt = time.Now()
		_, err := g.Memory.Update(ctx, r)
		require.NoError(t, err)

		_, there, err := g.stacks.Exists(ctx, w)
		require.NoError(t, err)

		assert.False(t, there)
	})

	t.Run("one there is no record of is no record", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t)

		ref, there, err := g.stacks.Exists(ctx, w)
		require.NoError(t, err)

		assert.False(t, there)
		assert.Zero(t, ref)
	})
}

func TestGraph_Restored(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	restoredAt := time.Now().Add(-time.Hour)

	// vm-1, its disk restored at restoredAt, first looked into since as
	// lookedAt says, a moment after, and heard of for as long as heard says
	// since: what lives in it was looked at, and nothing of it heard, then.
	restored := func(lookedAt time.Time, heard time.Duration) vmKind.VM {
		return vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.RestoredAt = restoredAt
			v.Status.LookedAt = lookedAt
			v.Status.ObservedAt = lookedAt.Add(heard)
		})
	}

	looked := restoredAt.Add(time.Second)

	// stopped and failed are vm-1 as v has it, but stopped, or failed, its
	// node heard of as long.
	stopped := func(v vmKind.VM) vmKind.VM {
		return vmtest.In(v, func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })
	}

	failed := func(v vmKind.VM) vmKind.VM {
		return vmtest.In(v, func(v *vmKind.VM) { v.Status.State = vmKind.Failed })
	}

	// vm-1 as its record is while a restore of it is carried out, before
	// that restore is taken onto it, its record saying what v says of a
	// restore before it.
	restoring := func(v vmKind.VM) vmKind.VM {
		return vmtest.In(v, func(v *vmKind.VM) { v.Status.State = vmKind.Restoring })
	}

	// a container of the stack s-1's in vm-1, reset by the vm's restore when
	// reset says, and no longer otherwise: found in the restored disk, or
	// taken in from it.
	container := func(uuid string, reset bool) resource.Record {
		return resource.Record{Raw: blockstest.AContainer(uuid, "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
			c.Metadata.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "s-1"}}
		}), Reset: reset}
	}

	// a moment after restoredAt: a restore of vm-1's after the one its
	// record says.
	later := restoredAt.Add(time.Hour)

	for name, tt := range map[string]struct {
		vms        []vmKind.VM
		containers []resource.Record

		// resetAt is the restore the stack was reset for: vm-1's at
		// restoredAt, unless it says another.
		resetAt  *time.Time
		restored graph.Restoration
	}{
		"held once its vm was looked into, and heard of for long enough since, when a container of it is no longer reset, found in the restored disk or taken in from it": {
			vms:        []vmKind.VM{restored(looked, silence+time.Second)},
			containers: []resource.Record{container("c-1", true), container("c-2", false)},
			restored:   graph.RestorationHeld,
		},
		"but not known before then, its vm not heard of for long enough since the look for every word of it to be heard, though one is": {
			vms:        []vmKind.VM{restored(looked, silence)},
			containers: []resource.Record{container("c-1", false)},
			restored:   graph.RestorationUnknown,
		},
		"not known while its vm was not looked into since its restore, however long its containers went unheard": {
			vms:        []vmKind.VM{restored(time.Time{}, 0)},
			containers: []resource.Record{container("c-1", true)},
			restored:   graph.RestorationUnknown,
		},
		"nor while it was looked into before its restore alone": {
			vms:      []vmKind.VM{restored(restoredAt.Add(-time.Minute), 2*silence)},
			restored: graph.RestorationUnknown,
		},
		"nor while its node was heard of it for no longer than what lives in it may go unheard since it was looked into, every word of that look not yet heard": {
			vms:        []vmKind.VM{restored(looked, silence)},
			containers: []resource.Record{container("c-1", true)},
			restored:   graph.RestorationUnknown,
		},
		"but not known still while a container of it is reset, which its own silence is yet to forget, or a heartbeat of it heard late to find": {
			vms:        []vmKind.VM{restored(looked, silence+time.Second)},
			containers: []resource.Record{container("c-1", true), container("c-2", true)},
			restored:   graph.RestorationUnknown,
		},
		"lost once none of its containers is left, each forgotten by its own silence, none found nor taken in": {
			vms:      []vmKind.VM{restored(looked, silence+time.Second)},
			restored: graph.RestorationLost,
		},
		"but not while its vm is stopped, which no container of it can be heard from, nor forgotten by its silence": {
			vms:        []vmKind.VM{stopped(restored(looked, silence+time.Second))},
			containers: []resource.Record{container("c-1", true)},
			restored:   graph.RestorationUnknown,
		},
		"nor though none of them is left": {
			vms:      []vmKind.VM{stopped(restored(looked, silence+time.Second))},
			restored: graph.RestorationUnknown,
		},
		"nor while its vm failed": {
			vms:      []vmKind.VM{failed(restored(looked, silence+time.Second))},
			restored: graph.RestorationUnknown,
		},
		"held when its vm is gone, which nothing looks into again: its graph makes it again": {
			containers: []resource.Record{container("c-1", true)},
			restored:   graph.RestorationHeld,
		},
		"and when its vm's record says nothing of a restore, nor its reset of one, made before resets said which": {
			vms:      []vmKind.VM{vmtest.Docker("vm-1", "owner")},
			resetAt:  &time.Time{},
			restored: graph.RestorationHeld,
		},
		"but not known while its vm's record says nothing of a restore, the one it was reset for, the vm's first, yet to be taken onto it": {
			vms:        []vmKind.VM{vmtest.Docker("vm-1", "owner")},
			containers: []resource.Record{container("c-1", true)},
			resetAt:    &restoredAt,
			restored:   graph.RestorationUnknown,
		},
		"nor while it says a restore before the one it was reset for, though it says that one was looked into, and heard of for long enough since, every container of it reset": {
			vms:        []vmKind.VM{restored(looked, silence+time.Second)},
			containers: []resource.Record{container("c-1", true), container("c-2", true)},
			resetAt:    &later,
			restored:   graph.RestorationUnknown,
		},
		"nor though one of its containers is no longer reset": {
			vms:        []vmKind.VM{restored(looked, silence+time.Second)},
			containers: []resource.Record{container("c-1", true), container("c-2", false)},
			resetAt:    &later,
			restored:   graph.RestorationUnknown,
		},
		"but not known while its vm's record says it is restoring, the restore that reset it not yet taken onto it, though it says nothing of one before": {
			vms:        []vmKind.VM{restoring(vmtest.Docker("vm-1", "owner"))},
			containers: []resource.Record{container("c-1", true)},
			restored:   graph.RestorationUnknown,
		},
		"nor though it says of the restore before that it was looked into, and heard of for long enough since, all of its containers reset": {
			vms:        []vmKind.VM{restoring(restored(looked, silence+time.Second))},
			containers: []resource.Record{container("c-1", true), container("c-2", true)},
			restored:   graph.RestorationUnknown,
		},
		"nor when one of its containers is no longer reset": {
			vms:        []vmKind.VM{restoring(restored(looked, silence+time.Second))},
			containers: []resource.Record{container("c-1", true), container("c-2", false)},
			restored:   graph.RestorationUnknown,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g := newStackGraph(t, vmtest.WithVMs(tt.vms...))
			g.Keep(aStack("s-1", "vm-1", shop))

			for _, c := range tt.containers {
				_, err := g.Memory.Create(ctx, c)
				require.NoError(t, err)
			}

			resetAt := restoredAt
			if tt.resetAt != nil {
				resetAt = *tt.resetAt
			}

			restoration, err := g.stacks.Restored(ctx, aStack("s-1", "vm-1", shop), resetAt)
			require.NoError(t, err)
			assert.Equal(t, tt.restored, restoration)
		})
	}

	t.Run("a container another stack owns says nothing of it", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(restored(looked, silence+time.Second)))
		g.Keep(aStack("s-1", "vm-1", shop))

		_, err := g.Memory.Create(ctx, resource.Record{Raw: blockstest.AContainer("c-other", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
			c.Metadata.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "s-2"}}
		})})
		require.NoError(t, err)

		restoration, err := g.stacks.Restored(ctx, aStack("s-1", "vm-1", shop), restoredAt)
		require.NoError(t, err)
		assert.Equal(t, graph.RestorationLost, restoration)
	})

	t.Run("a record of another kind is no stack's to say of", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t)

		_, err := g.stacks.Restored(ctx, blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running), restoredAt)
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})

	t.Run("what reading the records fails with is an error", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(restored(looked, silence+time.Second)))
		g.Memory.Fail = errors.New("the database is gone")

		_, err := g.stacks.Restored(ctx, aStack("s-1", "vm-1", shop), restoredAt)
		assert.ErrorIs(t, err, g.Memory.Fail)
	})
}

func TestGraph_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("its record says it is being removed, and stays, for its graph to remove last", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		g.Keep(aStack("s-1", "vm-1", twoServices, func(s *stackKind.Stack) { s.Status.State = stackKind.Running }))

		require.NoError(t, g.stacks.Delete(ctx, graph.Ref{Kind: stackKind.Name, UUID: "s-1"}))

		s, _, kept := storedStack(t, g, "s-1")
		require.True(t, kept)
		assert.Equal(t, stackKind.Removing, s.Status.State)

		require.NoError(t, g.stacks.Delete(ctx, graph.Ref{Kind: stackKind.Name, UUID: "s-1"}), "and again, which changes nothing")
		assert.Empty(t, g.Node.Sent(), "nothing is asked of any node")
	})

	t.Run("one gone already is removed", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, newStackGraph(t).stacks.Delete(ctx, graph.Ref{Kind: stackKind.Name, UUID: "s-gone"}))
	})

	t.Run("what is not a stack is not a stack's to remove", func(t *testing.T) {
		t.Parallel()

		assert.ErrorIs(t, newStackGraph(t).stacks.Delete(ctx, graph.Ref{Kind: containerKind.Name, UUID: "c-1"}), kind.ErrUnknownKind)
	})
}

// commands are the commands of the kinds named that a node was sent, as
// action kind/name of what each was sent of, in the order they were sent.
func commands(g *stackGraph, kinds ...string) []string {
	var sent []string

	for _, command := range g.Node.Sent() {
		if !slices.Contains(kinds, command.Kind) {
			continue
		}

		name := command.Resource.Metadata.Name

		sent = append(sent, command.Action+" "+command.Kind+"/"+name)
	}

	return sent
}

// applied is a stack of shop's in vm-1, kept deploying, once its graph, as
// policy types its edges, made what it needs and then it, with what kept
// says kept before, and the executor that made it.
func applied(t *testing.T, policy graph.DependencyPolicy, kept ...kind.Raw) (*stackGraph, *execute.Executor, *graph.Plan) {
	t.Helper()

	g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
	g.Keep(aStack("s-1", "vm-1", shop))

	for _, r := range kept {
		g.Keep(r)
	}

	executor, err := execute.New(g.kinds, ownership.New(g.Registry, g.Resources, nil), executionsMemory.NewRepository(), nil)
	require.NoError(t, err)

	plan, err := graph.NewPlanner(g.kinds, policy).Plan(wantOf(t, aStack("s-1", "vm-1", shop)).Requirement())
	require.NoError(t, err)

	outcome, err := executor.Apply(context.Background(), graph.Ref{Kind: stackKind.Name, UUID: "s-1"}, 1, plan)
	require.NoError(t, err)
	require.Equal(t, execute.Done, outcome.State, "%v", outcome.Err)

	return g, executor, plan
}

func TestGraph_applied(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := graph.Ref{Kind: stackKind.Name, UUID: "s-1"}

	t.Run("its graph makes its networks and its volume, then its services as they depend on one another, and then it, which runs", func(t *testing.T) {
		t.Parallel()

		g, _, _ := applied(t, graph.DefaultPolicy())

		made := commands(g, containerKind.Name, networkKind.Name, volumeKind.Name)
		require.Len(t, made, 8)

		assert.ElementsMatch(t, []string{"create network/shop_back", "create network/shop_default", "create network/shop_front", "create volume/shop_data"}, made[:4])
		assert.ElementsMatch(t, []string{"create container/shop_db", "create container/shop_worker"}, made[4:6])
		assert.Equal(t, []string{"create container/shop_api", "create container/shop_web"}, made[6:])

		s, r, _ := storedStack(t, g, "s-1")
		assert.Equal(t, stackKind.Running, s.Status.State)
		assert.Len(t, s.Status.Services, 4)
		assert.Contains(t, r.Metadata.Refs, kind.Reference{Kind: vmKind.Name, UUID: "vm-1"})

		db := blockRecord(t, g, containerKind.Name, "shop_db")
		assert.Equal(t, []kind.Reference{root}, db.Metadata.Owners, "its containers are its own")
		assert.Equal(t, "db", db.Metadata.Labels[stackKind.LabelService], "and say which of its services each is")

		back := blockRecord(t, g, networkKind.Name, "shop_back")
		assert.Len(t, back.Metadata.Owners, 4, "its network is owned by it and by what joins it")
		assert.Contains(t, back.Metadata.Owners, root)

		data := blockRecord(t, g, volumeKind.Name, "shop_data")
		assert.Empty(t, data.Metadata.Owners, "and its volume is used, and owned by nothing")
		assert.Contains(t, r.Metadata.Refs, kind.Reference{Kind: volumeKind.Name, UUID: data.Metadata.UUID})
	})

	t.Run("a network and a volume of its names that nothing owns, as what a restored disk holds is taken in, are made nothing of again, and are its own once its graph is carried out, as it would have made them", func(t *testing.T) {
		t.Parallel()

		taken := func(kindName string, uuid string, name string, spec any, status any) kind.Raw {
			raw, err := kind.Encode(kind.Resource[any, any]{
				Kind: kindName,
				Metadata: kind.Metadata{
					UUID:      uuid,
					Name:      name,
					OwnerUUID: "owner",
					Refs:      []kind.Reference{{Kind: vmKind.Name, UUID: "vm-1"}},
					Node:      vmtest.Node,
				},
				Spec:   spec,
				Status: status,
			})
			require.NoError(t, err)

			return raw
		}

		g, _, _ := applied(t, graph.DefaultPolicy(),
			taken(networkKind.Name, "n-back", "shop_back", networkKind.Spec{Name: "shop_back"}, networkKind.Status{Status: kind.Status{State: networkKind.Present, Expected: networkKind.Present}, Docker: &networkKind.Docker{ID: "n1", Name: "shop_back"}}),
			taken(volumeKind.Name, "v-data", "shop_data", volumeKind.Spec{Name: "shop_data"}, volumeKind.Status{Status: kind.Status{State: volumeKind.Present, Expected: volumeKind.Present}, Docker: &volumeKind.Docker{Name: "shop_data"}}),
		)

		made := commands(g, networkKind.Name, volumeKind.Name)
		assert.NotContains(t, made, "create network/shop_back", "found by its name, and made nothing of again")
		assert.NotContains(t, made, "create volume/shop_data")

		back := blockRecord(t, g, networkKind.Name, "shop_back")
		assert.Equal(t, "n-back", back.Metadata.UUID)
		assert.Len(t, back.Metadata.Owners, 4, "owned by it and by what joins it")
		assert.Contains(t, back.Metadata.Owners, root)

		data := blockRecord(t, g, volumeKind.Name, "shop_data")
		assert.Equal(t, "v-data", data.Metadata.UUID)
		assert.Empty(t, data.Metadata.Owners, "used, and owned by nothing, as a volume it made is")

		_, r, _ := storedStack(t, g, "s-1")
		assert.Contains(t, r.Metadata.Refs, kind.Reference{Kind: volumeKind.Name, UUID: "v-data"})
	})

	t.Run("removed, its containers go in the reverse of the order they depend on one another, then its networks, and its volume and its vm stay", func(t *testing.T) {
		t.Parallel()

		g, executor, plan := applied(t, graph.DefaultPolicy())
		before := len(commands(g, containerKind.Name, networkKind.Name, volumeKind.Name, vmKind.Name))

		outcome, err := executor.Remove(ctx, root, 1, plan)
		require.NoError(t, err)
		require.Equal(t, execute.Done, outcome.State, "%v", outcome.Err)

		removed := commands(g, containerKind.Name, networkKind.Name, volumeKind.Name, vmKind.Name)[before:]
		require.Len(t, removed, 7)

		assert.Equal(t, []string{"delete container/shop_web", "delete container/shop_api"}, removed[:2])
		assert.ElementsMatch(t, []string{"delete container/shop_db", "delete container/shop_worker"}, removed[2:4])
		assert.ElementsMatch(t, []string{"delete network/shop_back", "delete network/shop_default", "delete network/shop_front"}, removed[4:])

		_, _, kept := storedStack(t, g, "s-1")
		assert.False(t, kept, "its record went last of all")

		_, kept = findBlock(t, g, volumeKind.Name, "shop_data")
		assert.True(t, kept, "its volume stays")

		_, kept = g.Stored("vm-1")
		assert.True(t, kept, "and so does its vm")
	})

	t.Run("a network another root owns as well stays, unlinked from it", func(t *testing.T) {
		t.Parallel()

		g, executor, plan := applied(t, graph.DefaultPolicy())
		g.Keep(blockstest.AContainer("c-other", "vm-1", containerKind.Running, containerKind.Running))

		back := blockRecord(t, g, networkKind.Name, "shop_back")
		other := graph.Ref{Kind: containerKind.Name, UUID: "c-other"}
		require.NoError(t, ownership.New(g.Registry, g.Resources, nil).Link(ctx, graph.Ref{Kind: networkKind.Name, UUID: back.Metadata.UUID}, other, graph.Owned))

		outcome, err := executor.Remove(ctx, root, 1, plan)
		require.NoError(t, err)
		require.Equal(t, execute.Done, outcome.State, "%v", outcome.Err)

		assert.NotContains(t, commands(g, networkKind.Name), "delete network/shop_back")

		kept := blockRecord(t, g, networkKind.Name, "shop_back")
		assert.Equal(t, []kind.Reference{other}, kept.Metadata.Owners, "owned by what stays alone")

		_, there := findBlock(t, g, networkKind.Name, "shop_front")
		assert.False(t, there, "while what it owned alone went")
	})

	t.Run("with its volumes owned by it, its volume goes too, once what mounts it went", func(t *testing.T) {
		t.Parallel()

		owning := graph.DefaultPolicy().With(graph.DependencyPolicy{ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Owned}})

		g, executor, plan := applied(t, owning)
		before := len(commands(g, containerKind.Name, volumeKind.Name))

		outcome, err := executor.Remove(ctx, root, 1, plan)
		require.NoError(t, err)
		require.Equal(t, execute.Done, outcome.State, "%v", outcome.Err)

		removed := commands(g, containerKind.Name, volumeKind.Name)[before:]
		assert.Equal(t, "delete volume/shop_data", removed[len(removed)-1])
		assert.Less(t, slices.Index(removed, "delete container/shop_db"), slices.Index(removed, "delete volume/shop_data"))

		_, there := findBlock(t, g, volumeKind.Name, "shop_data")
		assert.False(t, there)
	})
}

// findBlock is the record of what of the named kind is kept in vm-1 under
// name, and whether one is.
func findBlock(t *testing.T, g *stackGraph, kindName string, name string) (resource.Record, bool) {
	t.Helper()

	kept, _, err := g.Resources.GetAll(context.Background(), kindName, resource.Filter{Parent: kind.Reference{Kind: vmKind.Name, UUID: "vm-1"}}, 0, 0)
	require.NoError(t, err)

	for _, r := range kept {
		if r.Metadata.Name == name {
			return r, true
		}
	}

	return resource.Record{}, false
}

// blockRecord is the record of what of the named kind is kept in vm-1 under
// name.
func blockRecord(t *testing.T, g *stackGraph, kindName string, name string) resource.Record {
	t.Helper()

	r, kept := findBlock(t, g, kindName, name)
	require.True(t, kept, "no %s is kept under %s", kindName, name)

	return r
}

// carrying has g's node carry out what it is sent at once, and say a
// container it stopped exited, as a node does.
func carrying(g *stackGraph) {
	g.Node.Answer(func(command kind.ActOnResource) (kind.ResourceActedOn, bool) {
		result, said := requirementstest.Carry(command)

		if command.Kind == containerKind.Name && command.Action == containerKind.ActionStop {
			c, _ := kind.Decode[containerKind.Spec, containerKind.Status](command.Resource)

			result.Status, _ = json.Marshal(containerKind.Status{
				Status:  kind.Status{State: containerKind.StateOf("exited", c.Spec.RestartPolicy)},
				Docker:  &containerKind.Docker{ID: "c-" + command.UUID, Name: c.Spec.Name, Image: c.Spec.Image, State: "exited", Status: "Exited (0) Less than a second ago"},
				Failure: &noderequest.Error{},
			})
		}

		return result, said
	})
}

// askStack has the stack s-1 of g asked for action, as its machine moves it
// and the dispatcher writes it down.
func askStack(t *testing.T, g *stackGraph, action string) {
	t.Helper()

	r, kept := g.Kept(stackKind.Name, "s-1")
	require.True(t, kept)

	binding, _ := g.Registry.Lookup(stackKind.Name)

	_, invalid, err := g.Dispatcher.Ask(context.Background(), binding, r, action, nil, true)
	require.NoError(t, err)
	require.Empty(t, invalid)
}

func TestGraph_actions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := graph.Ref{Kind: stackKind.Name, UUID: "s-1"}

	// carriedOut is the stack applied, once its plan is carried out again.
	carriedOut := func(t *testing.T, g *stackGraph, executor *execute.Executor, plan *graph.Plan) stackKind.Stack {
		t.Helper()

		outcome, err := executor.Apply(ctx, root, 1, plan)
		require.NoError(t, err)
		require.Equal(t, execute.Done, outcome.State, "%v", outcome.Err)

		s, _, _ := storedStack(t, g, "s-1")

		return s
	}

	t.Run("stopped, its containers stop in the reverse of the order they depend on one another, and are held stopped", func(t *testing.T) {
		t.Parallel()

		g, executor, plan := applied(t, graph.DefaultPolicy())
		carrying(g)
		before := len(commands(g, containerKind.Name))

		askStack(t, g, stackKind.ActionStop)

		s := carriedOut(t, g, executor, plan)
		assert.Equal(t, stackKind.Stopped, s.Status.State)
		assert.Equal(t, stackKind.Stopped, s.Status.Expected)

		stopped := commands(g, containerKind.Name)[before:]
		require.Len(t, stopped, 4)
		assert.Equal(t, []string{"stop container/shop_web", "stop container/shop_api"}, stopped[:2])
		assert.ElementsMatch(t, []string{"stop container/shop_db", "stop container/shop_worker"}, stopped[2:])

		for _, service := range []string{"web", "api", "db", "worker"} {
			c := blockRecord(t, g, containerKind.Name, "shop_"+service)
			common, err := c.Common()
			require.NoError(t, err)
			assert.Equal(t, containerKind.Stopped, common.Expected, "%s is to stay stopped, as any container stopped is", service)
		}

		again := carriedOut(t, g, executor, plan)
		assert.Equal(t, s.Status, again.Status, "carried out again, it is what it was")
		assert.Len(t, commands(g, containerKind.Name)[before:], 4, "and nothing more is asked")
	})

	t.Run("started, its containers start in the order they depend on one another, each level once the one before it runs", func(t *testing.T) {
		t.Parallel()

		g, executor, plan := applied(t, graph.DefaultPolicy())
		carrying(g)

		askStack(t, g, stackKind.ActionStop)
		carriedOut(t, g, executor, plan)

		before := len(commands(g, containerKind.Name))

		askStack(t, g, stackKind.ActionStart)

		s := carriedOut(t, g, executor, plan)
		assert.Equal(t, stackKind.Running, s.Status.State)
		assert.Equal(t, stackKind.Running, s.Status.Expected)

		started := commands(g, containerKind.Name)[before:]
		require.Len(t, started, 4)
		assert.ElementsMatch(t, []string{"start container/shop_db", "start container/shop_worker"}, started[:2])
		assert.Equal(t, []string{"start container/shop_api", "start container/shop_web"}, started[2:])
	})

	t.Run("restarted, they stop the one way and start the other", func(t *testing.T) {
		t.Parallel()

		g, executor, plan := applied(t, graph.DefaultPolicy())
		carrying(g)
		before := len(commands(g, containerKind.Name))

		askStack(t, g, stackKind.ActionRestart)

		s := carriedOut(t, g, executor, plan)
		assert.Equal(t, stackKind.Running, s.Status.State)

		restarted := commands(g, containerKind.Name)[before:]
		require.Len(t, restarted, 8)
		assert.Equal(t, []string{"stop container/shop_web", "stop container/shop_api"}, restarted[:2])
		assert.ElementsMatch(t, []string{"start container/shop_db", "start container/shop_worker"}, restarted[4:6])
		assert.Equal(t, []string{"start container/shop_api", "start container/shop_web"}, restarted[6:])
	})

	t.Run("a start cut short is taken up where it was left: what runs already is not started again", func(t *testing.T) {
		t.Parallel()

		g, executor, plan := applied(t, graph.DefaultPolicy())
		carrying(g)

		askStack(t, g, stackKind.ActionStop)
		carriedOut(t, g, executor, plan)

		// db and worker were started before the control plane stopped.
		for _, service := range []string{"db", "worker"} {
			c := blockRecord(t, g, containerKind.Name, "shop_"+service)
			_, err := g.Requirements.Ask(ctx, c, containerKind.ActionStart)
			require.NoError(t, err)
		}

		askStack(t, g, stackKind.ActionStart)
		before := len(commands(g, containerKind.Name))

		s := carriedOut(t, g, executor, plan)
		assert.Equal(t, stackKind.Running, s.Status.State)
		assert.Equal(t, []string{"start container/shop_api", "start container/shop_web"}, commands(g, containerKind.Name)[before:])
	})

	t.Run("those of a later level that do not run are held stopped before the first is started, so nothing starts them out of turn", func(t *testing.T) {
		t.Parallel()

		g, executor, plan := applied(t, graph.DefaultPolicy())
		carrying(g)

		askStack(t, g, stackKind.ActionStop)
		carriedOut(t, g, executor, plan)

		// web was asked to run since, and is stopped as docker left it.
		web := blockRecord(t, g, containerKind.Name, "shop_web")
		_, err := g.Dispatcher.Desire(ctx, web, containerKind.Running, true)
		require.NoError(t, err)

		var held []kind.State

		g.Node.Answer(func(command kind.ActOnResource) (kind.ResourceActedOn, bool) {
			if command.Kind == containerKind.Name && command.Action == containerKind.ActionStart && command.Resource.Metadata.Name == "shop_db" {
				w := blockRecord(t, g, containerKind.Name, "shop_web")
				common, _ := w.Common()
				held = append(held, common.Expected)
			}

			return requirementstest.Carry(command)
		})

		askStack(t, g, stackKind.ActionStart)
		carriedOut(t, g, executor, plan)

		assert.Equal(t, []kind.State{containerKind.Stopped}, held, "web was held stopped while db was started")
	})

	t.Run("a start a container fails stops there, and its plan fails, saying which", func(t *testing.T) {
		t.Parallel()

		g, executor, plan := applied(t, graph.DefaultPolicy())
		carrying(g)

		askStack(t, g, stackKind.ActionStop)
		carriedOut(t, g, executor, plan)

		g.Node.Answer(func(command kind.ActOnResource) (kind.ResourceActedOn, bool) {
			if command.Kind == containerKind.Name && command.Action == containerKind.ActionStart && command.Resource.Metadata.Name == "shop_api" {
				return requirementstest.Fail(command, "port is already allocated"), true
			}

			return requirementstest.Carry(command)
		})

		askStack(t, g, stackKind.ActionStart)
		before := len(commands(g, containerKind.Name))

		outcome, err := executor.Apply(ctx, root, 1, plan)
		require.NoError(t, err)
		require.Equal(t, execute.Failed, outcome.State)
		assert.ErrorContains(t, outcome.Err, "shop_api")
		assert.ErrorContains(t, outcome.Err, "port is already allocated")

		assert.NotContains(t, commands(g, containerKind.Name)[before:], "start container/shop_web", "what depends on it is not started")

		s, _, _ := storedStack(t, g, "s-1")
		assert.Equal(t, stackKind.Starting, s.Status.State, "its plan's failure is its reconciler's to say")
	})
}
