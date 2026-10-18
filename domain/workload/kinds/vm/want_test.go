package vm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestWant_Requirement(t *testing.T) {
	t.Parallel()

	want := vmKind.Want{OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: "vm-1", Name: "builds"}}

	required := want.Requirement()

	assert.Equal(t, graph.ID("vm", "vm-1"), required.ID(), "a vm is required by its uuid")
	assert.Equal(t, want, required.Spec, "and wanted as all of it")
	assert.False(t, required.External, "and is never external")
}

func TestChoiceOf(t *testing.T) {
	t.Parallel()

	v := vmKind.VM{
		Metadata: kind.Metadata{UUID: "vm-1", Name: "builds", Slug: "builds-abcde"},
		Spec: vmKind.Spec{
			Image:     "docker:29-dind",
			Resources: vmKind.Resources{CPUs: 2, Memory: 4 << 30, Disk: 20 << 30},
			Ports:     []port.Port{80, 443},
			Network:   vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		},
	}

	assert.Equal(t, stackKind.VMChoice{
		UUID:      "vm-1",
		Name:      "builds",
		Resources: &stackKind.Resources{CPUs: 2, Memory: 4 << 30, Disk: 20 << 30},
		Ports:     []port.Port{80, 443},
		Network:   &stackKind.Network{Ingress: "allow", Egress: "deny"},
		Slug:      "builds-abcde",
	}, vmKind.ChoiceOf(v), "what it is made with again, as its record has it, and where it is reached")

	v.Spec.Ports = nil

	choice := vmKind.ChoiceOf(v)
	assert.NotNil(t, choice.Ports, "and no ports are none, not the defaults")
	assert.Empty(t, choice.Ports)
	assert.True(t, choice.Describes())
}
