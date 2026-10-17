package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
)

func TestEdgeType(t *testing.T) {
	t.Parallel()

	for edge, said := range map[graph.EdgeType]string{
		graph.Owned:       "owned",
		graph.Loose:       "loose",
		graph.Order:       "order",
		graph.EdgeType(0): "unknown",
		graph.EdgeType(9): "unknown",
	} {
		assert.Equal(t, said, edge.String())
		assert.Equal(t, said != "unknown", edge.IsValid(), said)
	}
}

func TestDefaultPolicy(t *testing.T) {
	t.Parallel()

	policy := graph.DefaultPolicy()

	for name, tt := range map[string]struct {
		parent string
		child  string
		want   graph.EdgeType
	}{
		"a container's vm is loose":                    {parent: containerKind.Name, child: vmKind.Name, want: graph.Loose},
		"and so is a network's":                        {parent: networkKind.Name, child: vmKind.Name, want: graph.Loose},
		"and a volume it mounts":                       {parent: containerKind.Name, child: volumeKind.Name, want: graph.Loose},
		"a network it joins is owned":                  {parent: containerKind.Name, child: networkKind.Name, want: graph.Owned},
		"and a stack's container":                      {parent: stackKind.Name, child: containerKind.Name, want: graph.Owned},
		"and a stack's network":                        {parent: stackKind.Name, child: networkKind.Name, want: graph.Owned},
		"and anything of a kind it says nothing of":    {parent: stackKind.Name, child: "image", want: graph.Owned},
		"whatever requires a vm, it is loose":          {parent: "anything", child: vmKind.Name, want: graph.Loose},
		"whatever requires a kind it does not know of": {parent: "anything", child: "anything else", want: graph.Owned},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, policy.Type(tt.parent, tt.child))
		})
	}

	assert.Empty(t, policy.ByPair, "and no pair is given a type of its own")
}

func TestDependencyPolicy_Type(t *testing.T) {
	t.Parallel()

	policy := graph.DependencyPolicy{
		ByPair: map[[2]string]graph.EdgeType{
			{stackKind.Name, volumeKind.Name}: graph.Owned,
			{"service", "container"}:          graph.Order,
			{"broken", volumeKind.Name}:       graph.EdgeType(42),
		},
		ByKind: map[string]graph.EdgeType{
			vmKind.Name:     graph.Loose,
			volumeKind.Name: graph.Loose,
			"nothing":       graph.EdgeType(0),
		},
	}

	for name, tt := range map[string]struct {
		parent string
		child  string
		want   graph.EdgeType
	}{
		"what a pair is given beats what its required kind is given": {parent: stackKind.Name, child: volumeKind.Name, want: graph.Owned},
		"which is what any other dependent of the kind is given":     {parent: containerKind.Name, child: volumeKind.Name, want: graph.Loose},
		"any type may be given to a pair":                            {parent: "service", child: "container", want: graph.Order},
		"a pair is the dependent's kind, then the required kind's":   {parent: "container", child: "service", want: graph.Owned},
		"a kind given nothing is owned":                              {parent: containerKind.Name, child: networkKind.Name, want: graph.Owned},
		"and so is one given what is no type":                        {parent: containerKind.Name, child: "nothing", want: graph.Owned},
		"a pair given what is no type is what its kind is given":     {parent: "broken", child: volumeKind.Name, want: graph.Loose},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, policy.Type(tt.parent, tt.child))
		})
	}

	assert.Equal(t, graph.Owned, graph.DependencyPolicy{}.Type(containerKind.Name, vmKind.Name), "a policy that gives nothing owns everything")
}

func TestParsePolicy(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		text string
		want graph.DependencyPolicy
	}{
		"a kind owned": {
			text: "volume=owned",
			want: graph.DependencyPolicy{ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.Owned}},
		},
		"a kind loose": {
			text: "network=loose",
			want: graph.DependencyPolicy{ByKind: map[string]graph.EdgeType{networkKind.Name: graph.Loose}},
		},
		"a pair, the dependent's kind and then the kind it requires": {
			text: "stack/volume=owned",
			want: graph.DependencyPolicy{ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Owned}},
		},
		"kinds and pairs, separated by commas, in any order": {
			text: "stack/container=loose,volume=owned,container/network=loose,vm=owned",
			want: graph.DependencyPolicy{
				ByPair: map[[2]string]graph.EdgeType{
					{stackKind.Name, containerKind.Name}:   graph.Loose,
					{containerKind.Name, networkKind.Name}: graph.Loose,
				},
				ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.Owned, vmKind.Name: graph.Owned},
			},
		},
		"space around any part of an entry is not part of it": {
			text: "  volume = owned ,\tstack / volume=loose\n, vm =loose ",
			want: graph.DependencyPolicy{
				ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Loose},
				ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.Owned, vmKind.Name: graph.Loose},
			},
		},
		"a kind and a pair of it are two entries": {
			text: "volume=loose,stack/volume=owned",
			want: graph.DependencyPolicy{
				ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Owned},
				ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.Loose},
			},
		},
		"and so are a pair and the pair the other way round": {
			text: "stack/container=loose,container/stack=owned",
			want: graph.DependencyPolicy{ByPair: map[[2]string]graph.EdgeType{
				{stackKind.Name, containerKind.Name}: graph.Loose,
				{containerKind.Name, stackKind.Name}: graph.Owned,
			}},
		},
		"a kind of no kind the control plane runs, which nothing is typed by": {
			text: "image=owned",
			want: graph.DependencyPolicy{ByKind: map[string]graph.EdgeType{"image": graph.Owned}},
		},
		"nothing gives nothing":     {text: "", want: graph.DependencyPolicy{}},
		"and nor does space, alone": {text: " \t ", want: graph.DependencyPolicy{}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			policy, err := graph.ParsePolicy(tt.text)
			require.NoError(t, err)

			assert.Equal(t, tt.want, policy)
		})
	}
}

func TestParsePolicy_refused(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		text string
		said string
	}{
		"a type that is no type": {
			text: "vm=loose,volume=strong",
			said: `entry "volume=strong" gives "strong", which is no type: an edge is owned or loose`,
		},
		"nor one said otherwise": {
			text: "volume=Owned",
			said: `entry "volume=Owned" gives "Owned", which is no type: an edge is owned or loose`,
		},
		"nor none": {
			text: "volume=",
			said: `entry "volume=" gives "", which is no type: an edge is owned or loose`,
		},
		"an order, which only depends_on gives": {
			text: "stack/container=order",
			said: `entry "stack/container=order" gives an order, which only depends_on does`,
		},
		"nor to a kind": {
			text: "container=order",
			said: `entry "container=order" gives an order, which only depends_on does`,
		},
		"an entry that gives no type": {
			text: "vm=loose,volume",
			said: `entry "volume" is neither kind=type nor dependent/required=type`,
		},
		"nor one of three kinds": {
			text: "stack/container/network=owned",
			said: `entry "stack/container/network=owned" is neither kind=type nor dependent/required=type`,
		},
		"an entry that names no kind": {
			text: "=owned",
			said: `entry "=owned" names no kind`,
		},
		"nor a dependent": {
			text: "/volume=owned",
			said: `entry "/volume=owned" names no kind`,
		},
		"nor what is required": {
			text: "stack/ =owned",
			said: `entry "stack/ =owned" names no kind`,
		},
		"a kind that is no word": {
			text: "Volume=owned",
			said: `entry "Volume=owned" names "Volume", which is no kind: a kind's name is a lowercase word`,
		},
		"nor one with space in it": {
			text: "stack/my volume=owned",
			said: `entry "stack/my volume=owned" names "my volume", which is no kind: a kind's name is a lowercase word`,
		},
		"a kind given a type twice": {
			text: "volume=owned,vm=loose,volume=loose",
			said: `entry "volume=loose" gives volume a type an entry before it gave`,
		},
		"even the same one": {
			text: "volume=owned, volume = owned",
			said: `entry "volume = owned" gives volume a type an entry before it gave`,
		},
		"and a pair": {
			text: "stack/volume=owned,stack/volume=loose",
			said: `entry "stack/volume=loose" gives stack/volume a type an entry before it gave`,
		},
		"an empty entry": {
			text: "vm=loose,,volume=owned",
			said: "entry 2 is empty",
		},
		"nor a last one": {
			text: "vm=loose, ",
			said: "entry 2 is empty",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			policy, err := graph.ParsePolicy(tt.text)
			require.EqualError(t, err, tt.said)
			assert.Equal(t, graph.DependencyPolicy{}, policy, "and gives nothing")
		})
	}
}

func TestDependencyPolicy_String(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		policy graph.DependencyPolicy
		said   string
	}{
		"the defaults, each kind in the order of its name": {
			policy: graph.DefaultPolicy(),
			said:   "container=owned,network=owned,vm=loose,volume=loose",
		},
		"kinds, and then pairs, each in the order of its names": {
			policy: graph.DependencyPolicy{
				ByPair: map[[2]string]graph.EdgeType{
					{stackKind.Name, volumeKind.Name}:      graph.Owned,
					{containerKind.Name, volumeKind.Name}:  graph.Loose,
					{containerKind.Name, networkKind.Name}: graph.Loose,
				},
				ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.Owned, networkKind.Name: graph.Owned},
			},
			said: "network=owned,volume=owned,container/network=loose,container/volume=loose,stack/volume=owned",
		},
		"what is given that is no type is not said, as it is not given": {
			policy: graph.DependencyPolicy{
				ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.EdgeType(42)},
				ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.Owned, vmKind.Name: graph.EdgeType(0)},
			},
			said: "volume=owned",
		},
		"an order given in code is said": {
			policy: graph.DependencyPolicy{ByPair: map[[2]string]graph.EdgeType{{"service", containerKind.Name}: graph.Order}},
			said:   "service/container=order",
		},
		"and a policy that gives nothing says nothing": {
			policy: graph.DependencyPolicy{},
			said:   "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.said, tt.policy.String())
		})
	}

	t.Run("what is said is read back as the same policy", func(t *testing.T) {
		t.Parallel()

		for _, text := range []string{
			"",
			"volume=owned",
			"container=owned,network=owned,vm=loose,volume=loose",
			" stack / volume = owned , vm=owned,container/network=loose ",
			"stack/container=loose,container/stack=owned,image=owned",
		} {
			policy, err := graph.ParsePolicy(text)
			require.NoError(t, err)

			again, err := graph.ParsePolicy(policy.String())
			require.NoError(t, err, "%q", text)

			assert.Equal(t, policy, again, "%q", text)
			assert.Equal(t, policy.String(), again.String())
		}

		defaults, err := graph.ParsePolicy(graph.DefaultPolicy().String())
		require.NoError(t, err)
		assert.Equal(t, graph.DefaultPolicy(), defaults, "the defaults too")
	})
}

func TestDependencyPolicy_With(t *testing.T) {
	t.Parallel()

	t.Run("what is configured is laid over the defaults, entry by entry, and changes only what it names", func(t *testing.T) {
		t.Parallel()

		configured, err := graph.ParsePolicy("volume=owned")
		require.NoError(t, err)

		policy := graph.DefaultPolicy().With(configured)

		assert.Equal(t, graph.DependencyPolicy{ByKind: map[string]graph.EdgeType{
			vmKind.Name:        graph.Loose,
			volumeKind.Name:    graph.Owned,
			networkKind.Name:   graph.Owned,
			containerKind.Name: graph.Owned,
		}}, policy)

		assert.Equal(t, graph.Owned, policy.Type(containerKind.Name, volumeKind.Name), "a volume is owned")
		assert.Equal(t, graph.Loose, policy.Type(containerKind.Name, vmKind.Name), "and a vm still loose")
	})

	t.Run("nothing configured is the defaults, exactly", func(t *testing.T) {
		t.Parallel()

		nothing, err := graph.ParsePolicy("")
		require.NoError(t, err)

		assert.Equal(t, graph.DefaultPolicy(), graph.DefaultPolicy().With(nothing))
	})

	t.Run("a pair is laid beside the kinds, and is what its pair is given", func(t *testing.T) {
		t.Parallel()

		configured, err := graph.ParsePolicy("stack/volume=owned,network=loose")
		require.NoError(t, err)

		policy := graph.DefaultPolicy().With(configured)

		assert.Equal(t, graph.DependencyPolicy{
			ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Owned},
			ByKind: map[string]graph.EdgeType{
				vmKind.Name:        graph.Loose,
				volumeKind.Name:    graph.Loose,
				networkKind.Name:   graph.Loose,
				containerKind.Name: graph.Owned,
			},
		}, policy)

		assert.Equal(t, graph.Owned, policy.Type(stackKind.Name, volumeKind.Name), "a stack's volume is owned")
		assert.Equal(t, graph.Loose, policy.Type(containerKind.Name, volumeKind.Name), "and a container's still loose")
	})

	t.Run("what is laid over a pair takes its place, and no other pair's", func(t *testing.T) {
		t.Parallel()

		base := graph.DependencyPolicy{ByPair: map[[2]string]graph.EdgeType{
			{stackKind.Name, volumeKind.Name}:     graph.Owned,
			{containerKind.Name, volumeKind.Name}: graph.Owned,
		}}

		policy := base.With(graph.DependencyPolicy{ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Loose}})

		assert.Equal(t, graph.DependencyPolicy{ByPair: map[[2]string]graph.EdgeType{
			{stackKind.Name, volumeKind.Name}:     graph.Loose,
			{containerKind.Name, volumeKind.Name}: graph.Owned,
		}}, policy)
	})

	t.Run("what is laid over that is no type is not given, and changes nothing", func(t *testing.T) {
		t.Parallel()

		policy := graph.DefaultPolicy().With(graph.DependencyPolicy{
			ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.EdgeType(42)},
			ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.EdgeType(0)},
		})

		assert.Equal(t, graph.DefaultPolicy(), policy)
	})

	t.Run("neither policy is changed", func(t *testing.T) {
		t.Parallel()

		base := graph.DefaultPolicy()
		over := graph.DependencyPolicy{
			ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Owned},
			ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.Owned},
		}

		laid := base.With(over)
		laid.ByKind[vmKind.Name] = graph.Owned
		laid.ByPair[[2]string{stackKind.Name, volumeKind.Name}] = graph.Loose

		assert.Equal(t, graph.DefaultPolicy(), base)
		assert.Equal(t, graph.DependencyPolicy{
			ByPair: map[[2]string]graph.EdgeType{{stackKind.Name, volumeKind.Name}: graph.Owned},
			ByKind: map[string]graph.EdgeType{volumeKind.Name: graph.Owned},
		}, over)
	})
}
