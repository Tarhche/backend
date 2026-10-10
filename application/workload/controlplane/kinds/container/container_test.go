package container_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
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
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// asked is a container asked for as admission hands it over: in the vm in
// refs, when they name one.
func asked(spec containerKind.Spec, refs ...kind.Reference) containerKind.Container {
	return containerKind.Container{
		Kind:     containerKind.Name,
		Metadata: kind.Metadata{OwnerUUID: "owner", Refs: refs},
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

	t.Run("into the docker vm it names, which it belongs to and is placed where, keeping what to make it with again", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{Name: "web", Image: " nginx:1.27 "}, kind.Reference{Kind: "vm", UUID: "vm-1"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, admitted.Metadata.Refs, "it lives in its vm")
		assert.Empty(t, admitted.Metadata.Owners, "which does not own it")
		assert.Equal(t, vmtest.Node, admitted.Metadata.Node)
		assert.Equal(t, "web", admitted.Metadata.Name)
		assert.Equal(t, "nginx:1.27", admitted.Spec.Image)
		assert.Equal(t, stackKind.VMChoice{
			UUID:      "vm-1",
			Name:      "box",
			Resources: &stackKind.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB},
			Ports:     []port.Port{},
			Network:   &stackKind.Network{Ingress: "allow", Egress: "allow"},
			Slug:      "box-vm-1",
		}, admitted.Spec.VM, "its uuid, and its name, resources, ports, network and slug as its record has them")
		assert.Equal(t, containerKind.Pending, admitted.Status.Status.State, "for its graph to make")
		assert.Empty(t, admitted.Status.Reason)
		assert.Equal(t, containerKind.Running, admitted.Status.Expected)
	})

	t.Run("with what docker is asked beyond the rest as it was asked for", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		spec := containerKind.Spec{
			Image:      "nginx:1.27",
			Ports:      []containerKind.PortBinding{{ContainerPort: 80, HostPort: 8080, HostIP: "127.0.0.1", Protocol: "tcp"}, {ContainerPort: 443, HostIP: "::1", Protocol: "tcp"}},
			PidsLimit:  100,
			Privileged: true,
			User:       "1000:1000",
			Runtime:    "a-runtime-docker-says-whether-it-has",
		}

		admitted, invalid, err := w.Containers.Admit(ctx, asked(spec, kind.Reference{Kind: "vm", UUID: "vm-1"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		spec.VM = vmKind.ChoiceOf(vmtest.Docker("vm-1", "owner"))
		assert.Equal(t, spec, admitted.Spec, "a runtime is its vm's dockerd's to refuse")
	})

	t.Run("into the vm its spec names, whatever else names one, beside what else it uses and what owns it as they were given", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))

		c := asked(containerKind.Spec{Image: "nginx", VM: stackKind.VMChoice{UUID: "vm-1"}}, kind.Reference{Kind: "vm", UUID: "vm-2"}, kind.Reference{Kind: "volume", UUID: "data-uuid"})
		c.Metadata.Owners = []kind.Reference{{Kind: "stack", UUID: "stack-uuid"}}

		admitted, invalid, err := w.Containers.Admit(ctx, c)
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}, {Kind: "volume", UUID: "data-uuid"}}, admitted.Metadata.Refs, "one vm, the one it went into")
		assert.Equal(t, []kind.Reference{{Kind: "stack", UUID: "stack-uuid"}}, admitted.Metadata.Owners)
		assert.Equal(t, "vm-1", containerKind.VMOf(admitted))
	})

	t.Run("into one its graph is to make when it names none, though its owner has a docker vm already, which is kept nowhere yet", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{Image: "nginx", External: []string{"volume/data"}}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		planned := containerKind.VMOf(admitted)

		parsed, err := uuid.FromString(planned)
		require.NoError(t, err, "a uuid of its own, given it now")
		assert.Equal(t, byte(7), parsed.Version())
		assert.NotEqual(t, "vm-1", planned)

		_, kept := w.Stored(planned)
		assert.False(t, kept, "its graph makes it")
		assert.Equal(t, 1, w.Memory.Len(vmKind.Name), "and nothing else is made")
		assert.Empty(t, w.Producer.Messages())

		assert.Equal(t, stackKind.VMChoice{
			UUID:      planned,
			Name:      "docker",
			Resources: &stackKind.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB},
			Ports:     []port.Port{80},
			Network:   &stackKind.Network{Ingress: "allow", Egress: "allow"},
		}, admitted.Spec.VM, "kept with what to make it with, the defaults filled in")
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: planned}}, admitted.Metadata.Refs, "it lives in it already")
		assert.Empty(t, admitted.Metadata.Node, "and is on no node until it is")
		assert.Equal(t, containerKind.Pending, admitted.Status.Status.State)
		assert.Equal(t, "its vm is being made", admitted.Status.Reason)
		assert.Empty(t, admitted.Spec.External, "nothing is in a vm still to be made")
	})

	t.Run("into one its graph is to make as it describes it, the defaults filled in for what it leaves out", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New()

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{Image: "nginx", VM: stackKind.VMChoice{Name: "builds", Ports: []port.Port{}, Resources: &stackKind.Resources{Memory: 4 * vmtest.GiB}}}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		_, kept := w.Stored(containerKind.VMOf(admitted))
		assert.False(t, kept)

		assert.Equal(t, stackKind.VMChoice{
			UUID:      containerKind.VMOf(admitted),
			Name:      "builds",
			Resources: &stackKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB},
			Ports:     []port.Port{},
			Network:   &stackKind.Network{Ingress: "allow", Egress: "allow"},
		}, admitted.Spec.VM, "ports given empty are none")
	})

	t.Run("one whose vm its graph is to make is held to what its owner's vms are planned to be given besides", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		// a container whose vm is still to be made, of four vcpus.
		w.Keep(blockstest.AContainer("waiting", "planned-vm", containerKind.Pending, containerKind.Running, func(c *containerKind.Container) {
			c.Spec.VM = stackKind.VMChoice{UUID: "planned-vm", Resources: &stackKind.Resources{CPUs: 4}}
			c.Metadata.Node = ""
		}))

		_, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{Image: "nginx", VM: stackKind.VMChoice{Resources: &stackKind.Resources{CPUs: 3}}}))
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"vm.resources.cpus": "quota_exceeded"}, invalid)
	})

	t.Run("one in a docker vm it names keeps as not its own what its vm has already", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		present := func(uuid string, kindName string, name string, changes ...func(r *kind.Raw)) {
			r := blockstest.A[map[string]string, map[string]any](kindName, uuid, "vm-1", map[string]string{"name": name}, map[string]any{"state": "present", "expected": "present", "name": name})
			for _, change := range changes {
				change(&r)
			}

			w.Keep(r)
		}

		present("n-1", networkKind.Name, "backend")
		present("n-2", networkKind.Name, "leaving", func(r *kind.Raw) { r.Metadata.DeletionRequestedAt = time.Now() })
		present("v-1", volumeKind.Name, "data")

		sighted := func(kindName string, name string) {
			w.Sightings.Saw(kindName, blocks.Seen{Node: vmtest.Node, At: time.Now(), Manifest: kind.Raw{
				Kind:     kindName,
				Metadata: kind.Metadata{UUID: kindName + "-" + name, Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
				Status:   json.RawMessage(`{"state":"present","name":"` + name + `"}`),
			}})
		}

		sighted(networkKind.Name, "shared")
		sighted(volumeKind.Name, "cache")

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{
			Image:    "nginx",
			Networks: []string{"bridge", "backend", "frontend", "shared", "leaving", "backend"},
			Mounts: []containerKind.Mount{
				{Type: containerKind.MountVolume, Source: "data", Target: "/data"},
				{Type: containerKind.MountVolume, Source: "fresh", Target: "/fresh"},
				{Type: containerKind.MountVolume, Source: "cache", Target: "/cache"},
				{Type: containerKind.MountBind, Source: "/srv", Target: "/srv"},
			},
			External: []string{"network/frontend"},
		}, kind.Reference{Kind: "vm", UUID: "vm-1"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, []string{"network/backend", "network/shared", "volume/data", "volume/cache"}, admitted.Spec.External,
			"what its records keep and its dockerd holds with nobody keeping it, but not what is on its way out, what it was asked to say, or what every dockerd has")
	})

	t.Run("and one something owns keeps what it was asked to say of it", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.A[networkKind.Spec, networkKind.Status](networkKind.Name, "n-1", "vm-1", networkKind.Spec{Name: "backend"}, networkKind.Status{Status: kind.Status{State: networkKind.Present}}))

		c := asked(containerKind.Spec{Image: "nginx", Networks: []string{"backend"}, External: []string{"network/elsewhere"}}, kind.Reference{Kind: "vm", UUID: "vm-1"})
		c.Metadata.Owners = []kind.Reference{{Kind: "stack", UUID: "stack-uuid"}}

		admitted, invalid, err := w.Containers.Admit(ctx, c)
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, []string{"network/elsewhere"}, admitted.Spec.External, "its graph's to say")
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
		"one whose vm its graph is to make would be given more than one vm may be": {
			spec: containerKind.Spec{Image: "nginx", VM: stackKind.VMChoice{Resources: &stackKind.Resources{CPUs: 64}}},
			want: domain.ValidationErrors{"vm.resources.cpus": "too_large"},
		},
		"or more than its owner's vms may be given between them": {
			vms:  []vmKind.VM{vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner"), vmtest.Docker("vm-3", "owner")},
			spec: containerKind.Spec{Image: "nginx", VM: stackKind.VMChoice{Resources: &stackKind.Resources{CPUs: 3}}},
			want: domain.ValidationErrors{"vm.resources.cpus": "quota_exceeded"},
		},
		"one published on what is not an address": {
			vms:  []vmKind.VM{vmtest.Docker("vm-1", "owner")},
			spec: containerKind.Spec{Image: "nginx", Ports: []containerKind.PortBinding{{ContainerPort: 80, HostIP: "127.0.0.1"}, {ContainerPort: 81, HostIP: "localhost"}}},
			want: domain.ValidationErrors{"ports[1].host_ip": "invalid_ip"},
		},
		"or on an address of one interface's zone": {
			vms:  []vmKind.VM{vmtest.Docker("vm-1", "owner")},
			spec: containerKind.Spec{Image: "nginx", Ports: []containerKind.PortBinding{{ContainerPort: 80, HostIP: "fe80::1%eth0"}}},
			want: domain.ValidationErrors{"ports[0].host_ip": "invalid_ip"},
		},
		"one held to fewer processes than none": {
			vms:  []vmKind.VM{vmtest.Docker("vm-1", "owner")},
			spec: containerKind.Spec{Image: "nginx", PidsLimit: -1},
			want: domain.ValidationErrors{"pids_limit": "invalid_value"},
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
		"one not made yet, in a vm that runs, is its graph's to make": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Pending, containerKind.Running),
		},
		"and so is one its vm lost": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Missing, containerKind.Running),
		},
		"even one that is to be stopped, which is stopped once it is made": {
			container: blockstest.AContainer("c", "vm-1", containerKind.Missing, containerKind.Stopped),
		},
		"and one in a vm that does not run": {
			container: blockstest.AContainer("c", "vm-stopped", containerKind.Pending, containerKind.Running),
		},
		"or in one that is gone": {
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

	t.Run("put on a network, its spec names it, with the names it is reached by there, as not its own", func(t *testing.T) {
		t.Parallel()

		prepared, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "backend", Aliases: []string{"api"}})
		require.NoError(t, err)

		assert.Equal(t, []string{"backend"}, prepared.Spec.Networks)
		assert.Equal(t, map[string][]string{"backend": {"api"}}, prepared.Spec.Aliases)
		assert.Equal(t, []string{"network/backend"}, prepared.Spec.External, "it was there before it was put on it")

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
		assert.Nil(t, taken.Spec.External)
	})

	t.Run("a kept spec naming its vm and what to make it with again is no request, and is not refused for asking for two vms", func(t *testing.T) {
		t.Parallel()

		kept := typed(t, blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
			c.Spec.VM = vmKind.ChoiceOf(vmtest.Docker("vm-1", "owner"))
		}))

		prepared, invalid, err := w.Containers.Prepare(ctx, kept, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "backend"})
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, kept.Spec.VM, prepared.Spec.VM, "it is kept as it was")

		seen := *prepared.Status.Docker
		seen.Networks = []string{"backend", "bridge"}
		prepared.Status.Docker = &seen

		taken, invalid, err := w.Containers.Prepare(ctx, prepared, containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: "backend"})
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, kept.Spec.VM, taken.Spec.VM)
	})

	t.Run("one its graph made for it is its own", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.A[networkKind.Spec, networkKind.Status](networkKind.Name, "n-uuid", "vm-1", networkKind.Spec{Name: "backend"}, networkKind.Status{
			Status: kind.Status{State: networkKind.Present},
			Docker: &networkKind.Docker{ID: "0a1b2c3d", Name: "backend"},
		}))

		r, _ := w.Kept(networkKind.Name, "n-uuid")
		r.Metadata.Owners = []kind.Reference{{Kind: containerKind.Name, UUID: "c"}}
		_, err := w.Memory.Update(ctx, r)
		require.NoError(t, err)

		prepared, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "backend"})
		require.NoError(t, err)

		assert.Equal(t, []string{"backend"}, prepared.Spec.Networks)
		assert.Empty(t, prepared.Spec.External)
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

func TestContainers_Outlive(t *testing.T) {
	t.Parallel()

	w := blockstest.New()

	// the vm it named, renamed and resized since, as its record had it as it
	// went.
	updated := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
		v.Metadata.Name = "renamed"
		v.Metadata.Slug = "box-abcde"
		v.Spec.Resources = vmKind.Resources{CPUs: 3, Memory: 3 * vmtest.GiB, Disk: 20 * vmtest.GiB}
	})

	c := typed(t, blockstest.AContainer("c-1", "vm-1", kind.Missing, containerKind.Running))

	parent, err := kind.Encode(updated)
	require.NoError(t, err)

	t.Run("a container kept as gone with its vm keeps its vm as the vm's record had it as it went, and its slug", func(t *testing.T) {
		t.Parallel()

		kept, err := w.Containers.Outlive(c, parent)
		require.NoError(t, err)

		assert.Equal(t, vmKind.ChoiceOf(updated), kept.Spec.VM)
		assert.Equal(t, "box-abcde", kept.Spec.VM.Slug)
		assert.Equal(t, "renamed", kept.Spec.VM.Name)
	})

	t.Run("but a parent that is not its vm says nothing of it", func(t *testing.T) {
		t.Parallel()

		another, err := kind.Encode(vmtest.Docker("vm-2", "owner"))
		require.NoError(t, err)

		kept, err := w.Containers.Outlive(c, another)
		require.NoError(t, err)
		assert.Equal(t, c.Spec.VM, kept.Spec.VM)

		kept, err = w.Containers.Outlive(c, kind.Raw{Kind: "house", Metadata: kind.Metadata{UUID: "vm-1"}})
		require.NoError(t, err)
		assert.Equal(t, c.Spec.VM, kept.Spec.VM)
	})
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
