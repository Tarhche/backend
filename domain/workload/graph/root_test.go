package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

func TestRoots_Root(t *testing.T) {
	t.Parallel()

	containers := kind.Descriptor{Name: "container", Parent: "vm"}
	networks := kind.Descriptor{Name: "network", Parent: "vm"}
	stacks := kind.Descriptor{Name: "stack"}

	in := func(owners ...kind.Reference) kind.Metadata {
		return kind.Metadata{Refs: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, Owners: owners}
	}

	roots := graph.Roots{"container", "stack"}

	for name, tt := range map[string]struct {
		roots      graph.Roots
		descriptor kind.Descriptor
		metadata   kind.Metadata
		root       bool
	}{
		"one of a kind of root that nothing owns":                   {roots: roots, descriptor: containers, metadata: in(), root: true},
		"and one kept before refs, which lives among its owners":    {roots: roots, descriptor: containers, metadata: in(kind.Reference{Kind: "vm", UUID: "vm-1"}), root: true},
		"and one of a kind that lives in nothing, owned by nothing": {roots: roots, descriptor: stacks, metadata: kind.Metadata{}, root: true},
		"not one something owns":                                    {roots: roots, descriptor: containers, metadata: in(kind.Reference{Kind: "stack", UUID: "stack-1"})},
		"nor one of a kind that is no root":                         {roots: roots, descriptor: networks, metadata: in()},
		"nor anything with no kinds of root":                        {descriptor: containers, metadata: in()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.root, tt.roots.Root(tt.descriptor, tt.metadata))
		})
	}
}

func TestRelay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := graph.Ref{Kind: "container", UUID: "c-1"}

	t.Run("it tells nobody before it is given whom to tell", func(t *testing.T) {
		t.Parallel()

		var relay graph.Relay

		assert.NoError(t, relay.Changed(ctx, root))
		relay.Enqueue(root)
	})

	t.Run("and the graphs it is given from then on, failures and all", func(t *testing.T) {
		t.Parallel()

		var relay graph.Relay

		told := &graphtest.Told{}
		relay.To(told)

		require.NoError(t, relay.Changed(ctx, root))
		relay.Enqueue(root)

		told.Failure = errors.New("the database is gone")
		assert.ErrorIs(t, relay.Changed(ctx, graph.Ref{Kind: "network", UUID: "n-1"}), told.Failure)

		assert.Equal(t, []graph.Ref{root, {Kind: "network", UUID: "n-1"}}, told.Changes())
		assert.Equal(t, []graph.Ref{root}, told.Queued())
	})
}
