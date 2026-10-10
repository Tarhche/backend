package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
