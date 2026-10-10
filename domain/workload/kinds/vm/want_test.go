package vm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
)

func TestWant_Requirement(t *testing.T) {
	t.Parallel()

	want := vmKind.Want{OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: "vm-1", Name: "builds"}}

	required := want.Requirement()

	assert.Equal(t, graph.ID("vm", "vm-1"), required.ID(), "a vm is required by its uuid")
	assert.Equal(t, want, required.Spec, "and wanted as all of it")
	assert.False(t, required.External, "and is never external")
}
