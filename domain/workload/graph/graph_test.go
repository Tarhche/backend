package graph_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

func TestNodeID(t *testing.T) {
	t.Parallel()

	t.Run("a node is its kind and its name", func(t *testing.T) {
		t.Parallel()

		id := graph.ID("network", "shop_backend")

		assert.Equal(t, graph.NodeID("network/shop_backend"), id)
		assert.Equal(t, "network/shop_backend", id.String())
		assert.Equal(t, "network", id.Kind())
		assert.Equal(t, "shop_backend", id.Name())
	})

	for name, tt := range map[string]struct {
		id       graph.NodeID
		kindName string
		named    string
		ok       bool
	}{
		"a vm by its uuid":                       {id: "vm/0193b5c2-7b1e-7000-8000-000000000001", kindName: "vm", named: "0193b5c2-7b1e-7000-8000-000000000001", ok: true},
		"a name may have slashes of its own":     {id: "image/ghcr.io/acme/app:1", kindName: "image", named: "ghcr.io/acme/app:1", ok: true},
		"no slash names nothing":                 {id: "vm"},
		"nor does a kind with no name":           {id: "vm/"},
		"nor a name of no kind":                  {id: "/0193b5c2"},
		"nor nothing":                            {id: ""},
		"what is built is taken apart the same":  {id: graph.ID("container", "shop_web"), kindName: "container", named: "shop_web", ok: true},
		"and so is one named by its record uuid": {id: graph.ID("container", "c-uuid"), kindName: "container", named: "c-uuid", ok: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			kindName, named, ok := tt.id.Split()

			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.kindName, kindName)
			assert.Equal(t, tt.named, named)
			assert.Equal(t, tt.kindName, tt.id.Kind())
			assert.Equal(t, tt.named, tt.id.Name())
		})
	}
}

func TestRequirement_ID(t *testing.T) {
	t.Parallel()

	required := graph.Requirement{Kind: "volume", Name: "shop_data", External: true}

	assert.Equal(t, graph.NodeID("volume/shop_data"), required.ID(), "whatever else is said of it, it is its kind and its name")
}

func TestWaiting(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("the container web: %w", &graph.Waiting{Reason: "its vm is stopped"})

	var waiting *graph.Waiting
	require.ErrorAs(t, err, &waiting, "found however it is wrapped")
	assert.Equal(t, "its vm is stopped", waiting.Reason)
	assert.Equal(t, "the container web: waiting: its vm is stopped", err.Error())

	assert.False(t, errors.As(errors.New("its vm is stopped"), &waiting), "and nothing else is it")
}
