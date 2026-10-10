package volume_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := volume.Descriptor()

	assert.Empty(t, kind.Check(d), "it keeps every rule a kind keeps")
	assert.Equal(t, "vm", d.Parent)
	assert.Equal(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset}, d.OnParent)

	remove, _ := d.Action(volume.ActionDelete)
	admin, _ := d.Permissions(remove.Permission)
	assert.Equal(t, "workload.containers.delete", admin, "asked under the containers' permissions, as volumes always were")
}

func TestSpec_Docker(t *testing.T) {
	t.Parallel()

	spec := volume.Spec{Name: "data", Driver: "local", Labels: map[string]string{"team": "shop", "workload.volume": "forged"}}

	assert.Equal(t, docker.VolumeSpec{
		Name:   "data",
		Driver: "local",
		Labels: map[string]string{"team": "shop", "workload.managed": "true", "workload.volume": "v-uuid", volume.LabelRecreated: "2026-10-06T12:00:00Z"},
	}, spec.Docker("v-uuid", map[string]string{volume.LabelRecreated: "2026-10-06T12:00:00Z"}))
}

func TestReasonOf(t *testing.T) {
	t.Parallel()

	assert.Empty(t, volume.ReasonOf(nil), "one made once has nothing to say")
	assert.Empty(t, volume.ReasonOf(map[string]string{volume.LabelRecreated: "yesterday"}))

	reason := volume.ReasonOf(map[string]string{volume.LabelRecreated: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)})
	assert.Equal(t, "it went missing and was made again, empty, at 2026-10-06T12:00:00Z: what was in it is gone", reason)
}

func TestDocker(t *testing.T) {
	t.Parallel()

	held := docker.Volume{Name: "data", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data/_data", InUse: true}

	assert.Equal(t, held, volume.DockerOf(held).Volume())

	var none *volume.Docker
	assert.Equal(t, docker.Volume{}, none.Volume())
}

func TestStatus(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	status := volume.Status{
		Status: kind.Status{State: volume.Present, Expected: volume.Present, Reason: "it went missing and was made again", ObservedAt: at},
		Docker: &volume.Docker{
			Name:       "data",
			Driver:     "local",
			Mountpoint: "/var/lib/docker/volumes/data/_data",
			Labels:     map[string]string{"workload.managed": "true"},
			InUse:      true,
			CreatedAt:  at,
		},
		Failure: &noderequest.Error{Code: noderequest.CodeInvalid, Message: "volume is in use"},
	}

	written, err := json.Marshal(status)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"state": "present",
		"expected": "present",
		"reason": "it went missing and was made again",
		"observed_at": "2026-10-06T12:00:00Z",
		"name": "data",
		"driver": "local",
		"mountpoint": "/var/lib/docker/volumes/data/_data",
		"labels": {"workload.managed": "true"},
		"in_use": true,
		"created_at": "2026-10-06T12:00:00Z",
		"failure": {"code": "invalid", "message": "volume is in use"}
	}`, string(written), "what docker said of it is beside its state")

	var read volume.Status
	require.NoError(t, json.Unmarshal(written, &read))
	assert.Equal(t, status, read)
}
