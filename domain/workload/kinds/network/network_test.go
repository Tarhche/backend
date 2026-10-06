package network_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
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
