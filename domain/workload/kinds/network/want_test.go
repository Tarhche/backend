package network_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

func TestWant_Requirement(t *testing.T) {
	t.Parallel()

	want := network.Want{OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: "vm-1"}, Spec: network.Spec{Name: "shop_backend", Internal: true}}

	required := want.Requirement()

	assert.Equal(t, graph.ID("network", "shop_backend"), required.ID(), "a network is required by its docker name")
	assert.Equal(t, want, required.Spec)
}
