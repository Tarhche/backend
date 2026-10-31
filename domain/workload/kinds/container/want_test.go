package container_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

func TestWant_Requirement(t *testing.T) {
	t.Parallel()

	named := container.Want{UUID: "c-uuid", OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: "vm-1"}, Spec: container.Spec{Name: "shop_web", Image: "nginx"}}
	assert.Equal(t, graph.ID("container", "shop_web"), named.Requirement().ID(), "a container is required by its docker name")
	assert.Equal(t, named, named.Requirement().Spec)

	unnamed := container.Want{UUID: "c-uuid", OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: "vm-1"}, Spec: container.Spec{Image: "nginx"}}
	assert.Equal(t, graph.ID("container", "c-uuid"), unnamed.Requirement().ID(), "or by its record's uuid when it has no name")
}

func TestSpec_External(t *testing.T) {
	t.Parallel()

	spec := container.Spec{Image: "nginx", Networks: []string{"proxy"}, External: []string{"network/proxy", "volume/certs"}}

	written, err := container.SpecLabel(spec)
	require.NoError(t, err)

	read, labelled := container.SpecFromLabels(map[string]string{container.LabelSpec: written})
	require.True(t, labelled)
	assert.Equal(t, spec.External, read.External, "what is not its own is kept with what it was made with")

	none, err := json.Marshal(container.Spec{Image: "nginx"})
	require.NoError(t, err)
	assert.NotContains(t, string(none), "external", "and nothing is said of it when there is none")

	assert.Equal(t, spec.Docker("c-uuid", nil), container.Spec{Image: "nginx", Networks: []string{"proxy"}}.Docker("c-uuid", nil), "and docker is never asked anything of it")
}
