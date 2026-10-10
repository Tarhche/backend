package graph_test

import (
	"context"
	"errors"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
)

// the nodes of a stack the tests plan, s: the Docker VM it is in, its network
// and its volume, three of its services, and the stack itself.
var (
	vm      = graph.ID(vmKind.Name, "v-uuid")
	network = graph.ID(networkKind.Name, "s_default")
	volume  = graph.ID(volumeKind.Name, "s_data")
	web     = graph.ID(containerKind.Name, "s_web")
	api     = graph.ID(containerKind.Name, "s_api")
	db      = graph.ID(containerKind.Name, "s_db")
	stack   = graph.ID(stackKind.Name, "s-uuid")
)

func TestPlanner_Plan(t *testing.T) {
	t.Parallel()

	t.Run("a container is made after its vm, its network and its volume, and they after the vm", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()
		kinds.Require(web, requirement(vm), requirement(network), requirement(volume))
		kinds.Require(network, requirement(vm))
		kinds.Require(volume, requirement(vm))

		plan, err := planner(kinds).Plan(requirement(web))
		require.NoError(t, err)

		assert.Equal(t, [][]graph.NodeID{{vm}, {network, volume}, {web}}, plan.Levels())
	})

	t.Run("a stack's services are made in the order they depend_on one another, a level each", func(t *testing.T) {
		t.Parallel()

		plan, err := planner(services(map[graph.NodeID][]graph.NodeID{web: {api}, api: {db}})).Plan(requirement(stack))
		require.NoError(t, err)

		assert.Equal(t, [][]graph.NodeID{{vm}, {network, volume}, {db}, {api}, {web}, {stack}}, plan.Levels())

		for _, edge := range [][2]graph.NodeID{{db, api}, {api, web}} {
			e, found := plan.Edge(edge[0], edge[1])
			require.True(t, found, "%s is made before %s", edge[0], edge[1])
			assert.Equal(t, graph.Order, e.Type, "for the order alone")
		}
	})

	t.Run("one vm and one network are every service's, each asked what it requires once", func(t *testing.T) {
		t.Parallel()

		kinds := services(nil)

		plan, err := planner(kinds).Plan(requirement(stack))
		require.NoError(t, err)

		assert.Equal(t, [][]graph.NodeID{{vm}, {network, volume}, {api, db, web}, {stack}}, plan.Levels(), "services that depend_on nothing are made at once")
		assert.Len(t, plan.Nodes(), 7)

		asked := kinds.Asked(graphtest.Requires)
		assert.ElementsMatch(t, []graph.NodeID{stack, web, api, db, vm, network, volume}, asked)
		assert.Equal(t, stack, asked[0], "the root first")

		assert.Equal(t, []graph.Edge{
			{From: vm, To: api, Type: graph.Loose},
			{From: vm, To: db, Type: graph.Loose},
			{From: vm, To: web, Type: graph.Loose},
			{From: vm, To: network, Type: graph.Loose},
			{From: vm, To: volume, Type: graph.Loose},
		}, plan.Dependents(vm))

		assert.Equal(t, []graph.Edge{
			{From: network, To: api, Type: graph.Owned},
			{From: network, To: db, Type: graph.Owned},
			{From: network, To: web, Type: graph.Owned},
		}, plan.Dependents(network))
	})

	t.Run("no node is a vertex twice, however many need it", func(t *testing.T) {
		t.Parallel()

		plan, err := planner(services(map[graph.NodeID][]graph.NodeID{web: {api, db}, api: {db}})).Plan(requirement(stack))
		require.NoError(t, err)

		distinct := []graph.NodeID{vm, network, volume, web, api, db, stack}

		assert.Equal(t, uint32(len(distinct)), plan.Graph().Order())

		labels := make(map[graph.NodeID]int)
		for _, v := range plan.Graph().GetAllVertices() {
			labels[v.Label()]++
		}

		assert.Len(t, labels, len(distinct))
		for _, id := range distinct {
			assert.Equal(t, 1, labels[id], "%s is one vertex", id)
		}
	})

	t.Run("a root that requires nothing is a plan of its own", func(t *testing.T) {
		t.Parallel()

		plan, err := planner(graphtest.NewKind()).Plan(requirement(vm))
		require.NoError(t, err)

		assert.Equal(t, [][]graph.NodeID{{vm}}, plan.Levels())
		assert.Empty(t, plan.Edges())
		assert.Equal(t, uint32(1), plan.Graph().Order())
	})
}

func TestPlanner_Plan_edgeTypes(t *testing.T) {
	t.Parallel()

	// a container in a stack, joining a network and mounting a volume.
	kinds := func() *graphtest.Kind {
		kinds := graphtest.NewKind()
		kinds.Require(stack, requirement(web))
		kinds.Require(web, requirement(vm), requirement(network), requirement(volume))
		kinds.Require(network, requirement(vm))
		kinds.Require(volume, requirement(vm))

		return kinds
	}

	for name, tt := range map[string]struct {
		policy graph.DependencyPolicy
		want   []graph.Edge
	}{
		"by default, a vm and a volume are used, and a network and a container owned": {
			policy: graph.DefaultPolicy(),
			want: []graph.Edge{
				{From: web, To: stack, Type: graph.Owned},
				{From: network, To: web, Type: graph.Owned},
				{From: vm, To: web, Type: graph.Loose},
				{From: vm, To: network, Type: graph.Loose},
				{From: vm, To: volume, Type: graph.Loose},
				{From: volume, To: web, Type: graph.Loose},
			},
		},
		"a volume is owned by what mounts it when the policy says so, and no kind": {
			policy: graph.DependencyPolicy{ByKind: map[string]graph.EdgeType{vmKind.Name: graph.Loose, volumeKind.Name: graph.Owned}},
			want: []graph.Edge{
				{From: web, To: stack, Type: graph.Owned},
				{From: network, To: web, Type: graph.Owned},
				{From: vm, To: web, Type: graph.Loose},
				{From: vm, To: network, Type: graph.Loose},
				{From: vm, To: volume, Type: graph.Loose},
				{From: volume, To: web, Type: graph.Owned},
			},
		},
		"and a pair is what the policy gives it, whatever its required kind is given": {
			policy: graph.DependencyPolicy{
				ByPair: map[[2]string]graph.EdgeType{
					{stackKind.Name, containerKind.Name}:   graph.Loose,
					{containerKind.Name, networkKind.Name}: graph.Loose,
				},
				ByKind: graph.DefaultPolicy().ByKind,
			},
			want: []graph.Edge{
				{From: web, To: stack, Type: graph.Loose},
				{From: network, To: web, Type: graph.Loose},
				{From: vm, To: web, Type: graph.Loose},
				{From: vm, To: network, Type: graph.Loose},
				{From: vm, To: volume, Type: graph.Loose},
				{From: volume, To: web, Type: graph.Loose},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fake := kinds()

			plan, err := graph.NewPlanner(fake.Kinds(vmKind.Name, networkKind.Name, volumeKind.Name, containerKind.Name, stackKind.Name), tt.policy).Plan(requirement(stack))
			require.NoError(t, err)

			assert.Equal(t, tt.want, plan.Edges())
		})
	}

	t.Run("an order beside a requirement keeps the requirement's type, which is stronger", func(t *testing.T) {
		t.Parallel()

		fake := kinds()
		fake.Require(stack, requirement(web, volume, network))
		fake.Require(web, requirement(vm), requirement(network), requirement(volume, network))

		plan, err := planner(fake).Plan(requirement(stack))
		require.NoError(t, err)

		for edge, want := range map[[2]graph.NodeID]graph.EdgeType{
			{volume, web}:     graph.Loose,
			{network, web}:    graph.Owned,
			{network, volume}: graph.Order,
		} {
			e, found := plan.Edge(edge[0], edge[1])
			require.True(t, found, "%s is made before %s", edge[0], edge[1])
			assert.Equal(t, want, e.Type, "%s to %s", edge[0], edge[1])
		}
	})
}

func TestPlanner_Plan_merged(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		web      bool
		api      bool
		external bool
	}{
		"a network every service needing it says is not its own is not the graph's": {web: true, api: true, external: true},
		"one any of them does not say so of is the graph's":                         {web: true, api: false, external: false},
		"whichever needs it first":                                                  {web: false, api: true, external: false},
		"and so is one none says so of":                                             {external: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			byWeb := requirement(network)
			byWeb.External = tt.web

			byAPI := requirement(network)
			byAPI.External = tt.api

			kinds := graphtest.NewKind()
			kinds.Require(stack, requirement(web), requirement(api))
			kinds.Require(web, byWeb)
			kinds.Require(api, byAPI)

			plan, err := planner(kinds).Plan(requirement(stack))
			require.NoError(t, err)

			node, found := plan.Node(network)
			require.True(t, found)
			assert.Equal(t, tt.external, node.External)
		})
	}

	t.Run("a node is its first requirement, made after what any of them says", func(t *testing.T) {
		t.Parallel()

		byAPI := requirement(network, volume)
		byAPI.Spec = "the network as api says it"

		kinds := graphtest.NewKind()
		kinds.Require(stack, requirement(web), requirement(api), requirement(volume))
		kinds.Require(web, requirement(network))
		kinds.Require(api, byAPI)

		plan, err := planner(kinds).Plan(requirement(stack))
		require.NoError(t, err)

		node, found := plan.Node(network)
		require.True(t, found)
		assert.Equal(t, network, node.Spec, "web's, which needs it first")
		assert.Equal(t, []graph.NodeID{volume}, node.After, "after what api says too")

		assert.Equal(t, [][]graph.NodeID{{volume}, {network}, {api, web}, {stack}}, plan.Levels())
		assert.Equal(t, 1, countOf(kinds.Asked(graphtest.Requires), network), "asked what it requires as web's alone")
	})
}

func TestPlanner_Plan_cycle(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		after map[graph.NodeID][]graph.NodeID
		ring  []graph.NodeID
		said  string
	}{
		"services that depend_on one another round to the first": {
			after: map[graph.NodeID][]graph.NodeID{web: {api}, api: {db}, db: {web}},
			ring:  []graph.NodeID{web, api, db},
			said:  "depends_on cycle: container/s_web → container/s_api → container/s_db → container/s_web",
		},
		"two that depend_on each other": {
			after: map[graph.NodeID][]graph.NodeID{web: {api}, api: {web}},
			ring:  []graph.NodeID{web, api},
			said:  "depends_on cycle: container/s_web → container/s_api → container/s_web",
		},
		"one that depends_on itself": {
			after: map[graph.NodeID][]graph.NodeID{db: {db}},
			ring:  []graph.NodeID{db},
			said:  "depends_on cycle: container/s_db → container/s_db",
		},
		"and those in the ring alone, of a chain that runs into one": {
			after: map[graph.NodeID][]graph.NodeID{web: {api}, api: {db}, db: {api}},
			ring:  []graph.NodeID{db, api},
			said:  "depends_on cycle: container/s_db → container/s_api → container/s_db",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			plan, err := planner(services(tt.after)).Plan(requirement(stack))
			assert.Nil(t, plan)

			var cycle *graph.Cycle
			require.ErrorAs(t, err, &cycle)
			assert.Equal(t, tt.ring, cycle.Nodes)
			assert.EqualError(t, err, tt.said)
		})
	}

	t.Run("a ring is said the same way whatever order the services are said in", func(t *testing.T) {
		t.Parallel()

		for _, order := range [][]graph.NodeID{{web, api, db}, {web, db, api}, {api, web, db}, {api, db, web}, {db, web, api}, {db, api, web}} {
			kinds := services(nil)

			var required []graph.Requirement
			for _, id := range order {
				required = append(required, requirement(id, map[graph.NodeID]graph.NodeID{web: api, api: db, db: web}[id]))
			}

			kinds.Require(stack, required...)

			_, err := planner(kinds).Plan(requirement(stack))
			assert.EqualError(t, err, "depends_on cycle: container/s_web → container/s_api → container/s_db → container/s_web", "said in the order %v", order)
		}
	})

	t.Run("nodes that require one another are a kind's mistake, and no cycle of services", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()
		kinds.Require(stack, requirement(web))
		kinds.Require(web, requirement(stack))

		plan, err := planner(kinds).Plan(requirement(stack))
		assert.Nil(t, plan)
		require.EqualError(t, err, "requirement cycle: container/s_web → stack/s-uuid → container/s_web")

		var cycle *graph.Cycle
		assert.False(t, errors.As(err, &cycle))
	})
}

func TestPlanner_Plan_refused(t *testing.T) {
	t.Parallel()

	broken := errors.New("broken")

	every := []string{vmKind.Name, networkKind.Name, volumeKind.Name, containerKind.Name, stackKind.Name}

	for name, tt := range map[string]struct {
		root  graph.Requirement
		kinds func() *graphtest.Kind
		given []string
		is    error
		said  string
	}{
		"a root of a kind the planner is not given": {
			root:  requirement(stack),
			kinds: graphtest.NewKind,
			given: []string{vmKind.Name, containerKind.Name},
			is:    kind.ErrUnknownKind,
			said:  `unknown kind: "stack", the kind of stack/s-uuid`,
		},
		"or anything it requires": {
			root: requirement(web),
			kinds: func() *graphtest.Kind {
				kinds := graphtest.NewKind()
				kinds.Require(web, requirement(vm), graphtest.Requirement("image", "nginx:1"))

				return kinds
			},
			given: every,
			is:    kind.ErrUnknownKind,
			said:  `unknown kind: "image", the kind of image/nginx:1`,
		},
		"a service made after one that is not in the plan": {
			root: requirement(stack),
			kinds: func() *graphtest.Kind {
				return services(map[graph.NodeID][]graph.NodeID{web: {graph.ID(containerKind.Name, "s_cache")}})
			},
			given: every,
			said:  "container/s_web is made after container/s_cache, which is not in the plan",
		},
		"a spec its kind cannot read": {
			root:  graph.Requirement{Kind: containerKind.Name, Name: "s_web", Spec: "s_web"},
			kinds: graphtest.NewKind,
			given: every,
			is:    kind.ErrInvalidPayload,
			said:  "what container/s_web requires: invalid payload: a fake kind is required as a graph.NodeID, not as a string",
		},
		"what a kind fails to say": {
			root: requirement(web),
			kinds: func() *graphtest.Kind {
				kinds := graphtest.NewKind()
				kinds.Require(web, requirement(network))
				kinds.On(graphtest.Requires, network, func(context.Context) error { return broken })

				return kinds
			},
			given: every,
			is:    broken,
			said:  "what network/s_default requires: broken",
		},
		"a root that names no node": {
			root:  graph.Requirement{Kind: containerKind.Name},
			kinds: graphtest.NewKind,
			given: every,
			said:  `the root "container/" names no node`,
		},
		"or a requirement": {
			root: requirement(web),
			kinds: func() *graphtest.Kind {
				kinds := graphtest.NewKind()
				kinds.Require(web, requirement(vm), graphtest.Requirement(volumeKind.Name, ""))

				return kinds
			},
			given: every,
			said:  `container/s_web requires "volume/", which names no node`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			plan, err := graph.NewPlanner(tt.kinds().Kinds(tt.given...), graph.DefaultPolicy()).Plan(tt.root)
			assert.Nil(t, plan)

			require.EqualError(t, err, tt.said)
			if tt.is != nil {
				assert.ErrorIs(t, err, tt.is)
			}
		})
	}
}

func TestPlan(t *testing.T) {
	t.Parallel()

	// a container, made after its vm, its network, and its volume, which is
	// made after the network.
	kinds := graphtest.NewKind()
	kinds.Require(web, requirement(vm), requirement(network), requirement(volume, network))
	kinds.Require(network, requirement(vm))
	kinds.Require(volume, requirement(vm))

	plan, err := planner(kinds).Plan(requirement(web))
	require.NoError(t, err)

	t.Run("its root", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, web, plan.Root())
	})

	t.Run("a node by its id", func(t *testing.T) {
		t.Parallel()

		node, found := plan.Node(volume)
		require.True(t, found)
		assert.Equal(t, graph.Node{Requirement: graph.Requirement{Kind: volumeKind.Name, Name: "s_data", Spec: volume, After: []graph.NodeID{network}}}, node)

		_, found = plan.Node(api)
		assert.False(t, found, "and none of what it does not need")
	})

	t.Run("every node, in the order of their ids", func(t *testing.T) {
		t.Parallel()

		var ids []graph.NodeID
		for _, node := range plan.Nodes() {
			ids = append(ids, node.ID())
		}

		assert.Equal(t, []graph.NodeID{web, network, vm, volume}, ids)
	})

	t.Run("every edge, in the order of the nodes they are from, then of those they are to", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []graph.Edge{
			{From: network, To: web, Type: graph.Owned},
			{From: network, To: volume, Type: graph.Order},
			{From: vm, To: web, Type: graph.Loose},
			{From: vm, To: network, Type: graph.Loose},
			{From: vm, To: volume, Type: graph.Loose},
			{From: volume, To: web, Type: graph.Loose},
		}, plan.Edges())
	})

	t.Run("what a node needs, and how", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []graph.Edge{
			{From: network, To: web, Type: graph.Owned},
			{From: vm, To: web, Type: graph.Loose},
			{From: volume, To: web, Type: graph.Loose},
		}, plan.Dependencies(web))

		assert.Equal(t, []graph.Edge{
			{From: network, To: volume, Type: graph.Order},
			{From: vm, To: volume, Type: graph.Loose},
		}, plan.Dependencies(volume))

		assert.Empty(t, plan.Dependencies(vm), "and nothing of one that needs nothing")
	})

	t.Run("what needs a node, and how", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []graph.Edge{
			{From: vm, To: web, Type: graph.Loose},
			{From: vm, To: network, Type: graph.Loose},
			{From: vm, To: volume, Type: graph.Loose},
		}, plan.Dependents(vm))

		assert.Empty(t, plan.Dependents(web), "and nothing of one nothing needs")
	})

	t.Run("the edge from one node to another", func(t *testing.T) {
		t.Parallel()

		e, found := plan.Edge(vm, web)
		require.True(t, found)
		assert.Equal(t, graph.Edge{From: vm, To: web, Type: graph.Loose}, e)

		_, found = plan.Edge(web, vm)
		assert.False(t, found, "which is made before it, not after")

		_, found = plan.Edge(vm, api)
		assert.False(t, found, "and none to a node it does not have")
	})

	t.Run("what it answers is a copy of its own", func(t *testing.T) {
		t.Parallel()

		node, _ := plan.Node(volume)
		node.After[0] = api
		node.External = true

		nodes := plan.Nodes()
		nodes[3].After[0] = api

		levels := plan.Levels()
		levels[0][0] = api

		again, _ := plan.Node(volume)
		assert.Equal(t, []graph.NodeID{network}, again.After)
		assert.False(t, again.External)
		assert.Equal(t, [][]graph.NodeID{{vm}, {network}, {volume}, {web}}, plan.Levels())
	})
}

func TestPlan_Levels(t *testing.T) {
	t.Parallel()

	front := graph.ID(networkKind.Name, "s_front")
	cached := graph.ID(volumeKind.Name, "s_cache")
	cache := graph.ID(containerKind.Name, "s_cache")
	worker := graph.ID(containerKind.Name, "s_worker")

	// a stack of five services in one vm and on its network, web on another
	// too, and db and cache each mounting a volume: web is made after api and
	// cache, and api after db.
	required := map[graph.NodeID][]graph.Requirement{
		stack:   {requirement(web, api, cache), requirement(api, db), requirement(db), requirement(cache), requirement(worker)},
		web:     {requirement(vm), requirement(front), requirement(network)},
		api:     {requirement(vm), requirement(network)},
		db:      {requirement(vm), requirement(network), requirement(volume)},
		cache:   {requirement(vm), requirement(network), requirement(cached)},
		worker:  {requirement(vm), requirement(network)},
		front:   {requirement(vm)},
		network: {requirement(vm)},
		volume:  {requirement(vm)},
		cached:  {requirement(vm)},
	}

	want := [][]graph.NodeID{
		{vm},
		{network, front, cached, volume},
		{cache, db, worker},
		{api},
		{web},
		{stack},
	}

	// planned is the stack's plan, with what each node requires, and each of
	// those is made after, in the order shuffle leaves them in, when there is
	// one.
	planned := func(t *testing.T, shuffle *rand.Rand) *graph.Plan {
		kinds := graphtest.NewKind()

		for _, id := range slices.Sorted(maps.Keys(required)) {
			list := slices.Clone(required[id])

			if shuffle != nil {
				shuffle.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })

				for i := range list {
					after := slices.Clone(list[i].After)
					shuffle.Shuffle(len(after), func(i, j int) { after[i], after[j] = after[j], after[i] })
					list[i].After = after
				}
			}

			kinds.Require(id, list...)
		}

		plan, err := planner(kinds).Plan(requirement(stack))
		require.NoError(t, err)

		return plan
	}

	t.Run("a plan is made in one order, however many times it is planned", func(t *testing.T) {
		t.Parallel()

		for range 50 {
			assert.Equal(t, want, planned(t, nil).Levels())
		}
	})

	t.Run("whatever order its kinds say what they require in", func(t *testing.T) {
		t.Parallel()

		edges := planned(t, nil).Edges()
		shuffle := rand.New(rand.NewPCG(10, 10))

		for range 100 {
			plan := planned(t, shuffle)

			assert.Equal(t, want, plan.Levels())
			assert.Equal(t, edges, plan.Edges())
		}
	})
}

// planner plans graphs every kind of which is kinds, under the default
// policy.
func planner(kinds *graphtest.Kind) *graph.Planner {
	return graph.NewPlanner(kinds.Kinds(vmKind.Name, networkKind.Name, volumeKind.Name, containerKind.Name, stackKind.Name), graph.DefaultPolicy())
}

// requirement is the node id as a fake kind requires it, made after the nodes
// after names.
func requirement(id graph.NodeID, after ...graph.NodeID) graph.Requirement {
	r := graphtest.Requirement(id.Kind(), id.Name())
	r.After = after

	return r
}

// services is the stack s of web, api and db, each made after the services
// after says, all three in one Docker VM and on the stack's network, and db
// mounting its volume.
func services(after map[graph.NodeID][]graph.NodeID) *graphtest.Kind {
	kinds := graphtest.NewKind()
	kinds.Require(stack, requirement(web, after[web]...), requirement(api, after[api]...), requirement(db, after[db]...))
	kinds.Require(web, requirement(vm), requirement(network))
	kinds.Require(api, requirement(vm), requirement(network))
	kinds.Require(db, requirement(vm), requirement(network), requirement(volume))
	kinds.Require(network, requirement(vm))
	kinds.Require(volume, requirement(vm))

	return kinds
}

// countOf is how many times id is among ids.
func countOf(ids []graph.NodeID, id graph.NodeID) int {
	count := 0
	for _, each := range ids {
		if each == id {
			count++
		}
	}

	return count
}
