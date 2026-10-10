package container_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

func asked(spec containerKind.Spec, owners ...kind.Reference) containerKind.Container {
	return containerKind.Container{
		Kind:     containerKind.Name,
		Metadata: kind.Metadata{OwnerUUID: "owner", Owners: owners},
		Spec:     spec,
	}
}

func typed(t *testing.T, raw kind.Raw) containerKind.Container {
	t.Helper()

	c, err := kind.Decode[containerKind.Spec, containerKind.Status](raw)
	require.NoError(t, err)

	return c
}

func TestContainers_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("into the docker vm it names, which it belongs to and is placed where", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{Name: "web", Image: " nginx:1.27 "}, kind.Reference{Kind: "vm", UUID: "vm-1"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, admitted.Metadata.Owners)
		assert.Equal(t, vmtest.Node, admitted.Metadata.Node)
		assert.Equal(t, "web", admitted.Metadata.Name)
		assert.Equal(t, "nginx:1.27", admitted.Spec.Image)
		assert.Equal(t, stackKind.VMChoice{UUID: "vm-1"}, admitted.Spec.VM)
		assert.Equal(t, containerKind.Pending, admitted.Status.Status.State, "to be made once its vm runs")
		assert.Equal(t, containerKind.Running, admitted.Status.Expected)
	})

	t.Run("into one made for it when it names none, though its owner has a docker vm already", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{Image: "nginx"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		made, kept := w.Stored(containerKind.VMOf(admitted))
		require.True(t, kept)
		assert.NotEqual(t, "vm-1", made.Metadata.UUID)
		assert.True(t, vmKind.DockerVM(made, vmtest.Images.Docker))
		assert.Equal(t, stackKind.VMChoice{UUID: made.Metadata.UUID}, admitted.Spec.VM, "kept as the uuid of the vm it went into, and nothing else")
		assert.Equal(t, "its vm is scheduled", admitted.Status.Reason, "and it waits for it to come up")
	})

	t.Run("into one made for it as it describes it, which is kept as the vm's uuid alone", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New()

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{Image: "nginx", VM: stackKind.VMChoice{Name: "builds", Ports: []port.Port{}}}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		made, kept := w.Stored(containerKind.VMOf(admitted))
		require.True(t, kept)
		assert.Equal(t, "builds", made.Metadata.Name)
		assert.Empty(t, made.Spec.Ports, "ports given empty are none")
		assert.Equal(t, stackKind.VMChoice{UUID: made.Metadata.UUID}, admitted.Spec.VM)
	})

	for name, tt := range map[string]struct {
		vms   []vmKind.VM
		spec  containerKind.Spec
		owner []kind.Reference
		want  domain.ValidationErrors
	}{
		"one with no image": {
			vms:  []vmKind.VM{vmtest.Docker("vm-1", "owner")},
			spec: containerKind.Spec{Image: " "},
			want: domain.ValidationErrors{"image": "required_field"},
		},
		"one with a restart policy docker does not have": {
			vms:  []vmKind.VM{vmtest.Docker("vm-1", "owner")},
			spec: containerKind.Spec{Image: "nginx", RestartPolicy: "sometimes"},
			want: domain.ValidationErrors{"restart_policy": "invalid_restart_policy"},
		},
		"one naming a vm and describing a new one": {
			spec: containerKind.Spec{Image: "nginx", VM: stackKind.VMChoice{UUID: "vm-1", Name: "builds"}},
			want: domain.ValidationErrors{"vm": "vm_or_new_vm"},
		},
		"one in a docker vm that is stopped": {
			vms:   []vmKind.VM{vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })},
			spec:  containerKind.Spec{Image: "nginx"},
			owner: []kind.Reference{{Kind: "vm", UUID: "vm-1"}},
			want:  domain.ValidationErrors{"vm": "not_running"},
		},
		"one in a vm that is not a docker vm": {
			vms:   []vmKind.VM{vmtest.Running("vm-1", "owner")},
			spec:  containerKind.Spec{Image: "nginx"},
			owner: []kind.Reference{{Kind: "vm", UUID: "vm-1"}},
			want:  domain.ValidationErrors{"vm.uuid": "not_docker"},
		},
		"one in somebody else's": {
			vms:   []vmKind.VM{vmtest.Docker("vm-1", "somebody-else")},
			spec:  containerKind.Spec{Image: "nginx"},
			owner: []kind.Reference{{Kind: "vm", UUID: "vm-1"}},
			want:  domain.ValidationErrors{"vm.uuid": "not_found"},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			w := blockstest.New(vmtest.WithVMs(tt.vms...))

			_, invalid, err := w.Containers.Admit(ctx, asked(tt.spec, tt.owner...))
			require.NoError(t, err)

			assert.Equal(t, tt.want, invalid)
		})
	}
}

func TestContainers_Reconcile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stopped := vmtest.In(vmtest.Docker("vm-stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })
	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), stopped))

	onNetworks := func(networks ...string) func(c *containerKind.Container) {
		return func(c *containerKind.Container) {
			c.Spec.Networks = []string{"backend"}
			c.Spec.Aliases = map[string][]string{"backend": {"api"}}
			c.Status.Docker.Networks = networks
		}
	}

	for name, tt := range map[string]struct {
		container kind.Raw
		want      []kind.Intent
	}{
		"one not made yet, in a vm that runs, is made": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Pending, containerKind.Running),
			want:      []kind.Intent{{Action: containerKind.ActionCreate, Reason: "it is not in its vm, which runs"}},
		},
		"and so is one its vm lost": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Missing, containerKind.Running),
			want:      []kind.Intent{{Action: containerKind.ActionCreate, Reason: "it is not in its vm, which runs"}},
		},
		"even one that is to be stopped, which is stopped once it is made": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Missing, containerKind.Stopped),
			want:      []kind.Intent{{Action: containerKind.ActionCreate, Reason: "it is not in its vm, which runs"}},
		},
		"but not in a vm that does not run": {
			container: blockstest.AContainer("c", "vm-stopped", containerKind.Pending, containerKind.Running),
		},
		"nor in one that is gone": {
			container: blockstest.AContainer("c", "vm-gone", containerKind.Missing, containerKind.Running),
		},
		"one waiting on its vm waits": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Waiting, containerKind.Running),
		},
		"one that runs while it is to be stopped is stopped": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Stopped),
			want:      []kind.Intent{{Action: containerKind.ActionStop, Reason: "it runs while it was expected stopped"}},
		},
		"one stopped while it is to be running is started": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Stopped, containerKind.Running),
			want:      []kind.Intent{{Action: containerKind.ActionStart, Reason: "it stopped while it was expected running, and docker did not start it again"}},
		},
		"one that ran to its end is not drift": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Completed, containerKind.Running),
		},
		"one stopped and to be stopped is left": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Stopped, containerKind.Stopped),
		},
		"one that runs as it should is left": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Running),
		},
		"one off a network its spec names is put back on it": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Running, onNetworks("bridge")),
			want: []kind.Intent{{
				Action:  containerKind.ActionConnect,
				Payload: containerKind.ConnectPayload{Network: "backend", Aliases: []string{"api"}},
				Reason:  "it is not on backend, which its spec names",
			}},
		},
		"and one on it is left": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Running, onNetworks("backend", "bridge")),
		},
		"one to be deleted is the loop's to delete": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Running, kind.Deleted),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			intents, err := w.Containers.Reconcile(ctx, typed(t, tt.container))
			require.NoError(t, err)

			assert.Equal(t, tt.want, intents)
		})
	}
}

func TestContainers_Prepare(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
	w.Keep(blockstest.A[networkKind.Spec, networkKind.Status](networkKind.Name, "n-uuid", "vm-1", networkKind.Spec{Name: "backend"}, networkKind.Status{
		Status: kind.Status{State: networkKind.Present},
		Docker: &networkKind.Docker{ID: "0a1b2c3d", Name: "backend"},
	}))

	running := typed(t, blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Running))

	t.Run("one that runs is not removed but by force, in docker's words", func(t *testing.T) {
		t.Parallel()

		_, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionDelete, containerKind.DeletePayload{})

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, noderequest.CodeInvalid, refused.Code)
		assert.Equal(t, `cannot remove container "/web": container is running: stop the container before removing or force remove`, refused.Message)

		prepared, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionDelete, containerKind.DeletePayload{Force: true})
		require.NoError(t, err)
		assert.Equal(t, running, prepared)
	})

	t.Run("and one that does not run is", func(t *testing.T) {
		t.Parallel()

		exited := typed(t, blockstest.AContainer("c", "vm-1", containerKind.Stopped, containerKind.Stopped, func(c *containerKind.Container) {
			c.Status.Docker.State = "exited"
		}))

		_, _, err := w.Containers.Prepare(ctx, exited, containerKind.ActionDelete, containerKind.DeletePayload{})
		assert.NoError(t, err)
	})

	t.Run("put on a network, its spec names it, with the names it is reached by there", func(t *testing.T) {
		t.Parallel()

		prepared, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "backend", Aliases: []string{"api"}})
		require.NoError(t, err)

		assert.Equal(t, []string{"backend"}, prepared.Spec.Networks)
		assert.Equal(t, map[string][]string{"backend": {"api"}}, prepared.Spec.Aliases)

		taken, _, err := w.Containers.Prepare(ctx, prepared, containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: "backend"})
		require.ErrorAs(t, err, new(*noderequest.Error), "what docker has not done, it is not undone")
		assert.Empty(t, taken.Spec.Networks)

		// what its node saw since, on a copy of its own: the one before is
		// every other test's.
		seen := *prepared.Status.Docker
		seen.Networks = []string{"backend", "bridge"}
		prepared.Status.Docker = &seen

		taken, _, err = w.Containers.Prepare(ctx, prepared, containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: "backend"})
		require.NoError(t, err)

		assert.Empty(t, taken.Spec.Networks, "and taken off it, its spec names it no longer")
		assert.Nil(t, taken.Spec.Aliases)
	})

	t.Run("a network its vm has not is not found, and its spec is left as it was", func(t *testing.T) {
		t.Parallel()

		_, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "nowhere"})

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, noderequest.CodeNotFound, refused.Code)
	})

	t.Run("a network named by its docker id is named by its name in its spec, and taken off by it as well", func(t *testing.T) {
		t.Parallel()

		prepared, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "0a1b2c3d", Aliases: []string{"api"}})
		require.NoError(t, err)

		assert.Equal(t, []string{"backend"}, prepared.Spec.Networks)
		assert.Equal(t, map[string][]string{"backend": {"api"}}, prepared.Spec.Aliases)

		seen := *prepared.Status.Docker
		seen.Networks = []string{"backend", "bridge"}
		prepared.Status.Docker = &seen

		taken, _, err := w.Containers.Prepare(ctx, prepared, containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: "0a1b2c3d"})
		require.NoError(t, err)
		assert.Empty(t, taken.Spec.Networks)
		assert.Nil(t, taken.Spec.Aliases)
	})

	t.Run("one is not taken off a network its vm has not", func(t *testing.T) {
		t.Parallel()

		_, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: "nowhere"})

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, noderequest.CodeNotFound, refused.Code)
	})

	t.Run("one every dockerd has is there", func(t *testing.T) {
		t.Parallel()

		prepared, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "host"})
		require.NoError(t, err)
		assert.Equal(t, []string{"host"}, prepared.Spec.Networks)
	})

	t.Run("and one it is on already is refused, as docker would", func(t *testing.T) {
		t.Parallel()

		_, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "bridge"})
		assert.ErrorAs(t, err, new(*noderequest.Error))
	})
}

func TestContainers_Adopt(t *testing.T) {
	t.Parallel()

	w := blockstest.New()

	spec, err := containerKind.SpecLabel(containerKind.Spec{Name: "web", Image: "nginx:1.27", Env: []string{"A=1"}, RestartPolicy: containerKind.RestartNo})
	require.NoError(t, err)

	status := func(state kind.State, labels map[string]string) json.RawMessage {
		encoded, err := json.Marshal(containerKind.Status{Status: kind.Status{State: state}, Docker: &containerKind.Docker{ID: "c1", Name: "web", Labels: labels}})
		require.NoError(t, err)

		return encoded
	}

	adopted, ok := w.Containers.Adopt(kind.Observation{
		UUID:   "c-uuid",
		Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}},
		Status: status(containerKind.Completed, map[string]string{containerKind.LabelSpec: spec}),
	})
	require.True(t, ok)

	var read containerKind.Spec
	require.NoError(t, json.Unmarshal(adopted.Spec, &read))

	assert.Equal(t, "web", adopted.Name)
	assert.Equal(t, []string{"A=1"}, read.Env, "as it was asked for, which its label carries")
	assert.Equal(t, "vm-1", read.VM.UUID)
	assert.Equal(t, containerKind.Running, adopted.Expected, "a one-off that ended is still expected running")

	stoppedOne, ok := w.Containers.Adopt(kind.Observation{UUID: "c-uuid", Status: status(containerKind.Stopped, map[string]string{containerKind.LabelSpec: spec})})
	require.True(t, ok)
	assert.Equal(t, containerKind.Stopped, stoppedOne.Expected, "and one stopped is expected stopped")

	_, ok = w.Containers.Adopt(kind.Observation{UUID: "c-uuid", Status: status(containerKind.Running, nil)})
	assert.False(t, ok, "one whose spec its labels do not carry cannot be told")
}

func TestContainers_Named(t *testing.T) {
	t.Parallel()

	w := blockstest.New()

	assert.True(t, w.Containers.Named(blockstest.AContainer("c", "vm-1", containerKind.Pending, containerKind.Running), "web"), "by the name it was asked for with, before it is made")
	assert.False(t, w.Containers.Named(blockstest.AContainer("c", "vm-1", containerKind.Pending, containerKind.Running), "db"))
	assert.False(t, w.Containers.Named(kind.Raw{}, "web"))
}

func TestContainers_Apply(t *testing.T) {
	t.Parallel()

	_, _, err := blockstest.New().Containers.Apply(context.Background(), containerKind.Container{}, "rename", nil)
	assert.ErrorIs(t, err, kind.ErrUnknownAction)
}

// what is shared, the containers' own refusals among it, is held to the
// rules of the container kind.
var _ blocks.Kind = blockstest.New().Containers

// TestContainer_startAnswered holds a container being started to its start's
// answer: a look its node took between the start being asked and carried out
// says it is stopped, as it was, and is not what the start came to. Taken for
// it, the start's answer that it runs came for nothing anybody waited on, and
// the dashboard showed a container it had just started as exited.
func TestContainer_startAnswered(t *testing.T) {
	t.Parallel()

	d := containerKind.Descriptor()
	at := time.Now()

	r := resource.Record{Raw: blockstest.AContainer("web-uuid", "01", containerKind.Starting, containerKind.Running)}
	r.Pending = &resource.Pending{Action: containerKind.ActionStart, IDs: []string{"start-1"}, SentAt: at}

	stopped, err := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.Stopped}, Docker: &containerKind.Docker{ID: "c1", Name: "web", State: "exited"}})
	require.NoError(t, err)

	_, err = observe.Observe(d, &r, stopped, at.Add(time.Millisecond))
	require.NoError(t, err)

	common, err := r.Common()
	require.NoError(t, err)
	assert.Equal(t, containerKind.Starting, common.State, "a look before the start reached its node says nothing of it")
	require.NotNil(t, r.Pending, "and the start is still waited for")

	running, err := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.Running}, Docker: &containerKind.Docker{ID: "c1", Name: "web", State: "running"}})
	require.NoError(t, err)

	_, err = observe.Answer(d, &r, kind.ResourceActedOn{ID: "start-1", Kind: containerKind.Name, UUID: "web-uuid", Action: containerKind.ActionStart, OK: true, Status: running}, at.Add(2*time.Millisecond))
	require.NoError(t, err)

	common, err = r.Common()
	require.NoError(t, err)
	assert.Equal(t, containerKind.Running, common.State, "its answer says what it came to")
	assert.Nil(t, r.Pending)
}
