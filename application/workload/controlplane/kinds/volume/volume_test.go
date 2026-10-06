package volume_test

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
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func TestVolumes_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

	asked := func(spec volumeKind.Spec) volumeKind.Volume {
		return volumeKind.Volume{Kind: volumeKind.Name, Metadata: kind.Metadata{OwnerUUID: "owner", Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}}, Spec: spec}
	}

	admitted, invalid, err := w.Volumes.Admit(ctx, asked(volumeKind.Spec{Name: "data"}))
	require.NoError(t, err)
	require.Empty(t, invalid)

	assert.Equal(t, "data", admitted.Metadata.Name)
	assert.Equal(t, volumeKind.Pending, admitted.Status.State)
	assert.Equal(t, volumeKind.Present, admitted.Status.Expected)

	_, invalid, err = w.Volumes.Admit(ctx, asked(volumeKind.Spec{Name: "data", Driver: "nfs"}))
	require.NoError(t, err)
	assert.Equal(t, domain.ValidationErrors{"driver": "invalid_volume_driver"}, invalid)

	_, invalid, err = w.Volumes.Admit(ctx, asked(volumeKind.Spec{}))
	require.NoError(t, err)
	assert.Equal(t, domain.ValidationErrors{"name": "required_field"}, invalid)
}

func TestVolumes_Refuse(t *testing.T) {
	t.Parallel()

	w := blockstest.New()

	volume := func(inUse bool) kind.Raw {
		return blockstest.A[volumeKind.Spec, volumeKind.Status](volumeKind.Name, "v", "vm-1", volumeKind.Spec{Name: "data"}, volumeKind.Status{
			Status: kind.Status{State: volumeKind.Present},
			Docker: &volumeKind.Docker{Name: "data", InUse: inUse},
		})
	}

	var refused *noderequest.Error

	require.ErrorAs(t, w.Volumes.Refuse(context.Background(), volume(true), volumeKind.ActionDelete, volumeKind.DeletePayload{Force: true}), &refused)
	assert.Equal(t, "remove data: volume is in use", refused.Message, "one a container mounts, whatever the force, as docker says")

	assert.NoError(t, w.Volumes.Refuse(context.Background(), volume(false), volumeKind.ActionDelete, volumeKind.DeletePayload{}))
}

func TestVolumes_Adopt(t *testing.T) {
	t.Parallel()

	status, err := json.Marshal(volumeKind.Status{Status: kind.Status{State: volumeKind.Present}, Docker: &volumeKind.Docker{
		Name:   "data",
		Driver: "local",
		Labels: map[string]string{"workload.managed": "true", "workload.volume": "v-uuid", volumeKind.LabelRecreated: "2026-10-06T12:00:00Z"},
	}})
	require.NoError(t, err)

	adopted, ok := blockstest.New().Volumes.Adopt(kind.Observation{UUID: "v-uuid", Status: status})
	require.True(t, ok)

	var spec volumeKind.Spec
	require.NoError(t, json.Unmarshal(adopted.Spec, &spec))

	assert.Equal(t, volumeKind.Spec{Name: "data", Driver: "local"}, spec)
}
