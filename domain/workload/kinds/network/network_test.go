package network_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := network.Descriptor()

	assert.Empty(t, kind.Check(d), "it keeps every rule a kind keeps")
	assert.Equal(t, "vm", d.Parent)
	assert.Equal(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset}, d.OnParent)

	remove, _ := d.Action(network.ActionDelete)
	admin, self := d.Permissions(remove.Permission)
	assert.Equal(t, "workload.containers.delete", admin, "asked under the containers' permissions, as networks always were")
	assert.Equal(t, "self.workload.containers.delete", self)

	create, _ := d.Action(network.ActionCreate)
	assert.True(t, create.Internal)
}

func TestSpec_Docker(t *testing.T) {
	t.Parallel()

	spec := network.Spec{Name: "backend", Driver: "bridge", Internal: true, Labels: map[string]string{"team": "shop", "workload.network": "forged"}}

	assert.Equal(t, docker.NetworkSpec{
		Name:     "backend",
		Driver:   "bridge",
		Internal: true,
		Labels:   map[string]string{"team": "shop", "workload.managed": "true", "workload.network": "n-uuid"},
	}, spec.Docker("n-uuid"), "labelled as the platform's, and nobody else's labels say otherwise")
}

func TestDefault(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{"bridge": true, "host": true, "none": true, "backend": false} {
		assert.Equal(t, want, network.Default(name), name)
	}
}

func TestDocker(t *testing.T) {
	t.Parallel()

	held := docker.Network{ID: "n1", Name: "backend", Driver: "bridge", Scope: "local", Containers: []string{"web"}}

	assert.Equal(t, held, network.DockerOf(held).Network())
	assert.Equal(t, []string{}, network.DockerOf(docker.Network{}).Containers, "a list is a list even when it is empty")
}

func TestStatus(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	status := network.Status{
		Status: kind.Status{State: network.Present, Expected: network.Present, ObservedAt: at},
		Docker: &network.Docker{
			ID:         "n1",
			Name:       "backend",
			Driver:     "bridge",
			Scope:      "local",
			Internal:   true,
			Containers: []string{"web"},
			Labels:     map[string]string{"workload.managed": "true"},
			CreatedAt:  at,
		},
		Failure: &noderequest.Error{Code: noderequest.CodeInvalid, Message: "network backend has active endpoints"},
	}

	written, err := json.Marshal(status)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"state": "present",
		"expected": "present",
		"observed_at": "2026-10-06T12:00:00Z",
		"id": "n1",
		"name": "backend",
		"driver": "bridge",
		"scope": "local",
		"internal": true,
		"containers": ["web"],
		"labels": {"workload.managed": "true"},
		"created_at": "2026-10-06T12:00:00Z",
		"failure": {"code": "invalid", "message": "network backend has active endpoints"}
	}`, string(written), "what docker said of it is beside its state")

	var read network.Status
	require.NoError(t, json.Unmarshal(written, &read))
	assert.Equal(t, status, read)
}
