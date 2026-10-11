package container_test

import (
	"context"
	"encoding/json"
	"errors"
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

	t.Run("but what its vm's dockerd holds labelled as the platform's with no record only once its node has said it for long enough to show it: before then, it may be a heartbeat's ghost of what a delete removed, its graph's to make", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		// what its node said in a heartbeat or two, a moment ago, as one
		// taken before its delete removed it says, heard once its record
		// went; and what its node has said for long enough to show it, as
		// what outlived its record is.
		saying := func(kindName string, name string, from time.Time) {
			for at := from; !at.After(time.Now()); at = at.Add(time.Second) {
				w.Sightings.Heard(vmtest.Node, "vm-1", at)
				w.Sightings.Saw(kindName, blocks.Seen{Node: vmtest.Node, At: at, Labelled: true, Manifest: kind.Raw{
					Kind:     kindName,
					Metadata: kind.Metadata{UUID: kindName + "-" + name, Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
					Status:   json.RawMessage(`{"state":"present","name":"` + name + `"}`),
				}})
			}
		}

		saying(networkKind.Name, "deleted", time.Now().Add(-time.Second))
		saying(volumeKind.Name, "deleted-too", time.Now().Add(-time.Second))
		saying(networkKind.Name, "outlived", time.Now().Add(-blockstest.SureAfter-10*time.Second))
		saying(volumeKind.Name, "outlived-too", time.Now().Add(-blockstest.SureAfter-10*time.Second))

		require.Len(t, w.Sightings.Held(networkKind.Name, "vm-1"), 2, "held, both")
		require.Len(t, w.Sightings.In(networkKind.Name, "vm-1"), 1, "one shown")

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{
			Image:    "nginx",
			Networks: []string{"deleted", "outlived"},
			Mounts: []containerKind.Mount{
				{Type: containerKind.MountVolume, Source: "deleted-too", Target: "/data"},
				{Type: containerKind.MountVolume, Source: "outlived-too", Target: "/more"},
			},
		}, kind.Reference{Kind: "vm", UUID: "vm-1"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, []string{"network/outlived", "volume/outlived-too"}, admitted.Spec.External, "what was said for long enough to be shown is there before it, and what was said a moment ago is its graph's to make")
	})

	t.Run("and one shown in a vm whose dockerd stopped answering, as its node goes on saying what its other vms hold, until it is stale", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))

		// last said 35 s ago: not stale, and long enough ago for what its
		// node said since of another vm alone to leave it behind.
		start := time.Now().Add(-2*blockstest.SureAfter - 6*time.Second)

		outlived := kind.Raw{
			Kind:     networkKind.Name,
			Metadata: kind.Metadata{UUID: "outlived-uuid", Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
			Status:   json.RawMessage(`{"state":"present","name":"outlived"}`),
		}

		// said every second for as long as it waits to be shown, and a
		// moment more, with everything else its node says of vm-1.
		for at := start; !at.After(start.Add(blockstest.SureAfter + time.Second)); at = at.Add(time.Second) {
			w.Sightings.Heard(vmtest.Node, "vm-1", at)
			w.Sightings.Saw(networkKind.Name, blocks.Seen{Node: vmtest.Node, At: at, Labelled: true, Manifest: outlived})
		}

		require.Len(t, w.Sightings.In(networkKind.Name, "vm-1"), 1, "shown")

		// vm-1's dockerd answers no more, while vm-2's is heard up to now.
		w.Sightings.Heard(vmtest.Node, "vm-2", time.Now())

		admitted, invalid, err := w.Containers.Admit(ctx, asked(containerKind.Spec{
			Image:    "nginx",
			Networks: []string{"outlived"},
		}, kind.Reference{Kind: "vm", UUID: "vm-1"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, []string{"network/outlived"}, admitted.Spec.External, "its dockerd still holds it, as far as anyone heard")
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

	t.Run("one a stack owns is kept as its stack's service, as it was asked to say", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		c := asked(containerKind.Spec{Name: "shop_web", Image: "nginx"}, kind.Reference{Kind: "vm", UUID: "vm-1"})
		c.Metadata.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}
		c.Metadata.Labels = map[string]string{stackKind.LabelStack: "stack-uuid", stackKind.LabelService: "web", "team": "shop"}

		admitted, invalid, err := w.Containers.Admit(ctx, c)
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, map[string]string{stackKind.LabelStack: "stack-uuid", stackKind.LabelService: "web", "team": "shop"}, admitted.Metadata.Labels)
		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}, admitted.Metadata.Owners)
	})

	t.Run("but one no stack owns says it is nobody's service, whatever it was asked to say", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		c := asked(containerKind.Spec{Name: "web", Image: "nginx"}, kind.Reference{Kind: "vm", UUID: "vm-1"})
		c.Metadata.Labels = map[string]string{stackKind.LabelStack: "stack-uuid", stackKind.LabelService: "web", "team": "shop"}

		admitted, invalid, err := w.Containers.Admit(ctx, c)
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, map[string]string{"team": "shop"}, admitted.Metadata.Labels, "its dockerd would label it as the stack's otherwise")

		c.Metadata.Labels = map[string]string{stackKind.LabelService: "web"}

		admitted, invalid, err = w.Containers.Admit(ctx, c)
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Nil(t, admitted.Metadata.Labels)
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
		"even one that is to be stopped, which is made stopped": {
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
		require.NoError(t, err, "taken off it before its node's word that it is on it is on its record, as its spec names it")
		assert.Empty(t, taken.Spec.Networks)
		assert.Nil(t, taken.Spec.Aliases)
		assert.Nil(t, taken.Spec.External)

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

	t.Run("put on bridge, which its vm's dockerd has it on and its spec does not name, its spec names it, as no network there before it, and nothing more is asked of its node once it is on it", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []string{"bridge"}, running.Status.Docker.Networks)
		require.Empty(t, running.Spec.Networks)

		prepared, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "bridge"})
		require.NoError(t, err)

		assert.Equal(t, []string{"bridge"}, prepared.Spec.Networks)
		assert.Empty(t, prepared.Spec.External, "every dockerd has it, which its graph never makes or waits on")
		assert.Empty(t, prepared.Spec.Aliases)

		intents, err := w.Containers.Reconcile(ctx, prepared)
		require.NoError(t, err)
		assert.Empty(t, intents, "it is on it")

		_, _, err = w.Containers.Prepare(ctx, prepared, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "bridge"})
		assert.Error(t, err, "and put on it again is refused, as both its spec and docker say it is on it")
	})

	t.Run("but put on a network every dockerd has under aliases, which docker refuses there, it is refused, its spec as it was", func(t *testing.T) {
		t.Parallel()

		_, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "bridge", Aliases: []string{"api"}})

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, noderequest.CodeInvalid, refused.Code)
		assert.Equal(t, "network-scoped alias is supported only for containers in user defined networks", refused.Message)
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

	t.Run("one is taken off a network its spec names, or its vm's dockerd last said it is on, and refused as docker would only when neither does", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			spec    []string
			seen    []string
			unseen  bool
			refused bool
		}{
			"its spec names it, put on it a moment ago, or not put back on it, which its vm's dockerd has not said it is on": {spec: []string{"backend"}, seen: []string{"bridge"}},
			"its vm's dockerd said it is on it, though its spec does not name it":                                            {seen: []string{"backend", "bridge"}},
			"its spec names it, and its vm's dockerd has said nothing of it yet":                                             {spec: []string{"backend"}, unseen: true},
			"neither":                    {seen: []string{"bridge"}, refused: true},
			"nor, docker saying nothing": {unseen: true, refused: true},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				c := typed(t, blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
					c.Spec.Networks = tt.spec
					c.Status.Docker.Networks = tt.seen

					if tt.unseen {
						c.Status.Docker = nil
					}
				}))

				taken, _, err := w.Containers.Prepare(ctx, c, containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: "backend"})

				if tt.refused {
					var refused *noderequest.Error
					require.ErrorAs(t, err, &refused)
					assert.Equal(t, noderequest.CodeInvalid, refused.Code)
					assert.Equal(t, "container web is not connected to network backend", refused.Message, "in docker's words")

					return
				}

				require.NoError(t, err)
				assert.Empty(t, taken.Spec.Networks, "its spec names it no more")
			})
		}
	})

	t.Run("a network its vm's dockerd holds labelled as the platform's with no record is found at once, before it is shown", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		w.Sightings.Saw(networkKind.Name, blocks.Seen{Node: vmtest.Node, At: time.Now(), Labelled: true, Manifest: kind.Raw{
			Kind:     networkKind.Name,
			Metadata: kind.Metadata{UUID: "outlived-uuid", Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
			Status:   json.RawMessage(`{"state":"present","id":"9f8e7d6c","name":"outlived"}`),
		}})

		require.Empty(t, w.Sightings.In(networkKind.Name, "vm-1"), "not shown yet")

		put, _, err := w.Containers.Prepare(ctx, running, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "9f8e7d6c"})
		require.NoError(t, err, "put on it, by its docker id")
		assert.Equal(t, []string{"outlived"}, put.Spec.Networks, "named by its name")

		taken, _, err := w.Containers.Prepare(ctx, put, containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: "outlived"})
		require.NoError(t, err, "and taken off it")
		assert.Empty(t, taken.Spec.Networks)
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

	t.Run("one is put on a network its spec or its vm's dockerd does not say it is on, and refused as docker would only when both do", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			spec    []string
			seen    []string
			unseen  bool
			refused bool
		}{
			"its vm's dockerd last said it is on it, taken off it a moment ago, before that is on its record, its spec naming it no more": {seen: []string{"backend", "bridge"}},
			"its spec names it, put on it a moment ago, or not put back on it, which its vm's dockerd has not said it is on":              {spec: []string{"backend"}, seen: []string{"bridge"}},
			"its spec names it, and its vm's dockerd has said nothing of it yet":                                                          {spec: []string{"backend"}, unseen: true},
			"neither": {seen: []string{"bridge"}},
			"both":    {spec: []string{"backend"}, seen: []string{"backend", "bridge"}, refused: true},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				c := typed(t, blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
					c.Spec.Networks = tt.spec
					c.Status.Docker.Networks = tt.seen

					if tt.unseen {
						c.Status.Docker = nil
					}
				}))

				put, _, err := w.Containers.Prepare(ctx, c, containerKind.ActionConnect, containerKind.ConnectPayload{Network: "backend"})

				if tt.refused {
					var refused *noderequest.Error
					require.ErrorAs(t, err, &refused)
					assert.Equal(t, noderequest.CodeInvalid, refused.Code)
					assert.Equal(t, "endpoint with name web already exists in network backend", refused.Message, "in docker's words")

					return
				}

				require.NoError(t, err)
				assert.Equal(t, []string{"backend"}, put.Spec.Networks, "its spec names it")
			})
		}
	})
}

func TestContainers_Adopt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	spec, err := containerKind.SpecLabel(containerKind.Spec{Name: "web", Image: "nginx:1.27", Env: []string{"A=1"}, RestartPolicy: containerKind.RestartNo})
	require.NoError(t, err)

	status := func(state kind.State, labels map[string]string) json.RawMessage {
		encoded, err := json.Marshal(containerKind.Status{Status: kind.Status{State: state}, Docker: &containerKind.Docker{ID: "c1", Name: "web", Labels: labels}})
		require.NoError(t, err)

		return encoded
	}

	inVM1 := []kind.Reference{{Kind: "vm", UUID: "vm-1"}}

	t.Run("as it was asked for, which its labels carry, expected to be doing what it does", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New()

		adopted, ok, err := w.Containers.Adopt(ctx, kind.Observation{UUID: "c-uuid", Owners: inVM1, Status: status(containerKind.Completed, map[string]string{containerKind.LabelSpec: spec})})
		require.NoError(t, err)
		require.True(t, ok)

		var read containerKind.Spec
		require.NoError(t, json.Unmarshal(adopted.Spec, &read))

		assert.Equal(t, "web", adopted.Name)
		assert.Equal(t, []string{"A=1"}, read.Env, "as it was asked for, which its label carries")
		assert.Equal(t, "vm-1", read.VM.UUID)
		assert.Equal(t, containerKind.Running, adopted.Expected, "a one-off that ended is still expected running")
		assert.Empty(t, adopted.Owners, "nobody's")
		assert.Empty(t, adopted.Labels)

		stoppedOne, ok, err := w.Containers.Adopt(ctx, kind.Observation{UUID: "c-uuid", Status: status(containerKind.Stopped, map[string]string{containerKind.LabelSpec: spec})})
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, containerKind.Stopped, stoppedOne.Expected, "and one stopped is expected stopped")

		_, ok, err = w.Containers.Adopt(ctx, kind.Observation{UUID: "c-uuid", Status: status(containerKind.Running, nil)})
		require.NoError(t, err)
		assert.False(t, ok, "one whose spec its labels do not carry cannot be told")
	})

	// a stack's, in the vm stackVM names, under the slug shop-abcde.
	aStack := func(t *testing.T, uuid string, stackVM string) kind.Raw {
		t.Helper()

		raw, err := kind.Encode(stackKind.Stack{
			Kind:     stackKind.Name,
			Metadata: kind.Metadata{UUID: uuid, Name: "shop", Slug: "shop-abcde", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: "vm", UUID: stackVM}}},
			Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: stackVM}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
			Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running}},
		})
		require.NoError(t, err)

		return raw
	}

	stackLabels := func(stackUUID string) map[string]string {
		return map[string]string{containerKind.LabelSpec: spec, stackKind.LabelStack: stackUUID, stackKind.LabelService: "web"}
	}

	t.Run("one its labels say is a service of a stack in its vm is the stack's again, as the stack's graph makes the container of a service", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New()
		w.Keep(aStack(t, "stack-1", "vm-1"))

		adopted, ok, err := w.Containers.Adopt(ctx, kind.Observation{UUID: "c-uuid", Owners: inVM1, Status: status(containerKind.Running, stackLabels("stack-1"))})
		require.NoError(t, err)
		require.True(t, ok)

		var read containerKind.Spec
		require.NoError(t, json.Unmarshal(adopted.Spec, &read))

		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "stack-1"}}, adopted.Owners, "owned by its stack")
		assert.Equal(t, map[string]string{stackKind.LabelStack: "stack-1", stackKind.LabelService: "web"}, adopted.Labels, "labelled as the stack's, and as the service")
		assert.Equal(t, "shop-abcde_web", adopted.Name, "named under its stack's slug, as its stack's graph knows it")
		assert.Equal(t, "shop-abcde_web", read.Name)
		assert.Equal(t, "nginx:1.27", read.Image, "and as it was asked for")
		assert.Equal(t, "vm-1", read.VM.UUID)
	})

	for name, tt := range map[string]struct {
		stacks []kind.Raw
		labels map[string]string
	}{
		"one naming a stack that is not there": {
			labels: stackLabels("gone"),
		},
		"one naming a stack in another vm": {
			stacks: []kind.Raw{aStack(t, "stack-2", "vm-2")},
			labels: stackLabels("stack-2"),
		},
		"one naming no service of the stack": {
			stacks: []kind.Raw{aStack(t, "stack-1", "vm-1")},
			labels: map[string]string{containerKind.LabelSpec: spec, stackKind.LabelStack: "stack-1"},
		},
		"one naming a service the stack's file has not": {
			stacks: []kind.Raw{aStack(t, "stack-1", "vm-1")},
			labels: map[string]string{containerKind.LabelSpec: spec, stackKind.LabelStack: "stack-1", stackKind.LabelService: "not-in-its-file"},
		},
		"one naming a stack whose file cannot be read": {
			stacks: []kind.Raw{func() kind.Raw {
				s, err := kind.Decode[stackKind.Spec, stackKind.Status](aStack(t, "stack-1", "vm-1"))
				require.NoError(t, err)

				s.Spec.Compose = "services:\n  web:\n    image: nginx:1.27\n    build: .\n"

				raw, err := kind.Encode(s)
				require.NoError(t, err)

				return raw
			}()},
			labels: stackLabels("stack-1"),
		},
	} {
		t.Run(name+" is nobody's, as any other is: its labels confer nothing", func(t *testing.T) {
			t.Parallel()

			w := blockstest.New()
			for _, s := range tt.stacks {
				w.Keep(s)
			}

			adopted, ok, err := w.Containers.Adopt(ctx, kind.Observation{UUID: "c-uuid", Owners: inVM1, Status: status(containerKind.Running, tt.labels)})
			require.NoError(t, err)
			require.True(t, ok)

			assert.Empty(t, adopted.Owners)
			assert.Empty(t, adopted.Labels)
			assert.Equal(t, "web", adopted.Name, "as it was asked for")
		})
	}

	// a container record of stack-1's, named as its web server's, as changes
	// say otherwise.
	ofStack1 := func(uuid string, changes ...func(c *containerKind.Container)) kind.Raw {
		return blockstest.AContainer(uuid, "vm-1", containerKind.Running, containerKind.Running, append([]func(c *containerKind.Container){func(c *containerKind.Container) {
			c.Metadata.Name = "shop-abcde_web"
			c.Metadata.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "stack-1"}}
			c.Spec.Name = "shop-abcde_web"
			c.Status.Docker.Name = "shop-abcde_web"
		}}, changes...)...)
	}

	for name, tt := range map[string]struct {
		kept  resource.Record
		owned bool
	}{
		"a second container of a service whose container the stack has a record of, found in the restored disk or taken in from it, is nobody's: a stack's service has one container": {
			kept: resource.Record{Raw: ofStack1("found")},
		},
		"but one whose record is on its way out is the stack's": {
			kept:  resource.Record{Raw: ofStack1("going", func(c *containerKind.Container) { c.Status.Expected = kind.Deleted })},
			owned: true,
		},
		"and so is one whose record is reset, dockerd naming its container as it names this one, so that the restored disk does not hold it": {
			kept:  resource.Record{Raw: ofStack1("made-since-the-snapshot", func(c *containerKind.Container) { c.Status.Docker.Name = "web" }), Reset: true},
			owned: true,
		},
		"but not one whose record is reset, dockerd naming its container otherwise, which the restored disk may hold beside it": {
			kept: resource.Record{Raw: ofStack1("maybe-in-the-disk"), Reset: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := blockstest.New()
			w.Keep(aStack(t, "stack-1", "vm-1"))

			_, err := w.Memory.Create(ctx, tt.kept)
			require.NoError(t, err)

			adopted, ok, err := w.Containers.Adopt(ctx, kind.Observation{UUID: "c-uuid", Owners: inVM1, Status: status(containerKind.Running, stackLabels("stack-1"))})
			require.NoError(t, err)
			require.True(t, ok)

			if tt.owned {
				assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "stack-1"}}, adopted.Owners)
				assert.Equal(t, "shop-abcde_web", adopted.Name)

				return
			}

			assert.Empty(t, adopted.Owners)
			assert.Empty(t, adopted.Labels)
			assert.Equal(t, "web", adopted.Name, "named as it was asked for, not as the service's")
		})
	}

	t.Run("what reading its stack fails with is an error, and it is not taken in", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New()
		w.Memory.Fail = errors.New("the database is gone")

		_, ok, err := w.Containers.Adopt(ctx, kind.Observation{UUID: "c-uuid", Owners: inVM1, Status: status(containerKind.Running, stackLabels("stack-1"))})
		assert.ErrorIs(t, err, w.Memory.Fail)
		assert.False(t, ok)
	})
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
