package volume_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
)

func TestWant_Requirement(t *testing.T) {
	t.Parallel()

	want := volume.Want{OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: "vm-1"}, Spec: volume.Spec{Name: "shop_data"}}

	required := want.Requirement()

	assert.Equal(t, graph.ID("volume", "shop_data"), required.ID(), "a volume is required by its docker name")
	assert.Equal(t, want, required.Spec)
}
