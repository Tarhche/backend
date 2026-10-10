package network_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func TestNetworks_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

	asked := func(spec networkKind.Spec) networkKind.Network {
		return networkKind.Network{Kind: networkKind.Name, Metadata: kind.Metadata{OwnerUUID: "owner", Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}}, Spec: spec}
	}

	admitted, invalid, err := w.Networks.Admit(ctx, asked(networkKind.Spec{Name: " backend ", Internal: true}))
	require.NoError(t, err)
	require.Empty(t, invalid)

	assert.Equal(t, "backend", admitted.Spec.Name)
	assert.Equal(t, "backend", admitted.Metadata.Name)
	assert.True(t, admitted.Spec.Internal)
	assert.Equal(t, vmtest.Node, admitted.Metadata.Node)
	assert.Equal(t, networkKind.Pending, admitted.Status.State)
	assert.Equal(t, networkKind.Present, admitted.Status.Expected)

	for spec, want := range map[*networkKind.Spec]domain.ValidationErrors{
		{}:                                   {"name": "required_field"},
		{Name: "backend", Driver: "overlay"}: {"driver": "invalid_network_driver"},
	} {
		_, invalid, err := w.Networks.Admit(ctx, asked(*spec))
		require.NoError(t, err)
		assert.Equal(t, want, invalid)
	}
}

func TestNetworks_Refuse(t *testing.T) {
	t.Parallel()

	w := blockstest.New()

	network := func(name string, containers ...string) kind.Raw {
		return blockstest.A[networkKind.Spec, networkKind.Status](networkKind.Name, "n", "vm-1", networkKind.Spec{Name: name}, networkKind.Status{
			Status: kind.Status{State: networkKind.Present},
			Docker: &networkKind.Docker{ID: "n1", Name: name, Containers: containers},
		})
	}

	var refused *noderequest.Error

	assert.NoError(t, w.Networks.Refuse(context.Background(), network("backend", "web"), networkKind.ActionDelete, nil), "whether a container is on it is its dockerd's to say: what its node reported may be from before the container was taken off it")

	require.ErrorAs(t, w.Networks.Refuse(context.Background(), network("bridge"), networkKind.ActionDelete, nil), &refused)
	assert.Equal(t, "bridge is a pre-defined network and cannot be removed", refused.Message)

	assert.NoError(t, w.Networks.Refuse(context.Background(), network("backend"), networkKind.ActionDelete, nil))
}

func TestNetworks_Adopt(t *testing.T) {
	t.Parallel()

	status, err := json.Marshal(networkKind.Status{Status: kind.Status{State: networkKind.Present}, Docker: &networkKind.Docker{
		Name:     "backend",
		Driver:   "bridge",
		Internal: true,
		Labels:   map[string]string{"workload.managed": "true", "workload.network": "n-uuid", "team": "shop"},
	}})
	require.NoError(t, err)

	adopted, ok := blockstest.New().Networks.Adopt(kind.Observation{UUID: "n-uuid", Status: status})
	require.True(t, ok)

	var spec networkKind.Spec
	require.NoError(t, json.Unmarshal(adopted.Spec, &spec))

	assert.Equal(t, networkKind.Spec{Name: "backend", Driver: "bridge", Internal: true, Labels: map[string]string{"team": "shop"}}, spec, "as it is, but for the platform's own labels")
	assert.Equal(t, networkKind.Present, adopted.Expected)
}
