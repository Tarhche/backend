package vm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	controlPlaneVMs "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// asked is a machine somebody asks for, as admission is handed one, changed
// as changes say.
func asked(changes ...func(v *vmKind.VM)) vmKind.VM {
	v := vmKind.VM{
		Kind:     vmKind.Name,
		Metadata: kind.Metadata{Name: "My Box", OwnerUUID: "owner-uuid"},
		Spec: vmKind.Spec{
			Flavor:    vmKind.FlavorMachine,
			Resources: vmKind.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 10 * vmtest.GiB},
			Ports:     []port.Port{8080, 22, 8080},
			Network:   vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		},
	}

	for _, change := range changes {
		change(&v)
	}

	return v
}

// owned is a VM of owner-uuid's given resources, running on the node.
func owned(uuid string, resources vmKind.Resources) vmKind.VM {
	return vmtest.In(vmtest.Running(uuid, "owner-uuid"), func(v *vmKind.VM) { v.Spec.Resources = resources })
}

func TestVMs_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a vm is placed where there is room, made to run, with what it left out filled in", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		admitted, invalid, err := w.VMs.Admit(ctx, asked())
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, vmKind.Name, admitted.Kind)
		assert.Equal(t, "My Box", admitted.Metadata.Name)
		assert.True(t, strings.HasPrefix(admitted.Metadata.Slug, "my-box-"), admitted.Metadata.Slug)
		assert.Equal(t, "owner-uuid", admitted.Metadata.OwnerUUID)
		assert.Equal(t, vmtest.Node, admitted.Metadata.Node)
		assert.Equal(t, map[string]string{vmKind.LabelFlavor: "machine"}, admitted.Metadata.Labels, "its flavor is a label, which a listing narrows by")
		assert.Equal(t, "ubuntu:24.04", admitted.Spec.Image, "a machine that names no image boots the default one")
		assert.Equal(t, []port.Port{22, 8080}, admitted.Spec.Ports, "sorted, each once")
		assert.Equal(t, vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, admitted.Spec.Network)
		assert.Nil(t, admitted.Spec.Source)
		assert.Equal(t, vmKind.Created, admitted.Status.State)
		assert.Equal(t, vmKind.Running, admitted.Status.Expected)
		assert.Empty(t, admitted.Status.Reason)
	})

	t.Run("a docker vm boots from the docker image and nothing else", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		docker := asked(func(v *vmKind.VM) {
			v.Spec.Flavor = vmKind.FlavorDocker
			v.Spec.Resources = vmKind.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB}
		})

		admitted, invalid, err := w.VMs.Admit(ctx, docker)
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, "docker:29-dind", admitted.Spec.Image)
		assert.Equal(t, "docker", admitted.Metadata.Labels[vmKind.LabelFlavor])

		docker.Spec.Image = "alpine:3"

		_, invalid, err = w.VMs.Admit(ctx, docker)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"image": "invalid_image"}, invalid)
	})

	t.Run("a network left out is open both ways, and no ports are none", func(t *testing.T) {
		t.Parallel()

		admitted, invalid, err := vmtest.New().VMs.Admit(ctx, asked(func(v *vmKind.VM) {
			v.Spec.Network = vmKind.Network{}
			v.Spec.Ports = nil
		}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow}, admitted.Spec.Network)
		assert.Equal(t, []port.Port{}, admitted.Spec.Ports)
	})

	t.Run("its labels are kept, but being the code runner's is not its to say", func(t *testing.T) {
		t.Parallel()

		admitted, _, err := vmtest.New().VMs.Admit(ctx, asked(func(v *vmKind.VM) {
			v.Metadata.Labels = map[string]string{"team": "web", vmKind.LabelManagedBy: vmKind.ManagedByCodeRunner, vmKind.LabelFlavor: "docker"}
		}))
		require.NoError(t, err)

		assert.Equal(t, map[string]string{"team": "web", vmKind.LabelFlavor: "machine"}, admitted.Metadata.Labels)
	})

	t.Run("its lifetime is kept", func(t *testing.T) {
		t.Parallel()

		admitted, _, err := vmtest.New().VMs.Admit(ctx, asked(func(v *vmKind.VM) { v.Metadata.Lifetime = time.Hour }))
		require.NoError(t, err)

		assert.Equal(t, time.Hour, admitted.Metadata.Lifetime)
	})

	for name, tt := range map[string]struct {
		change func(v *vmKind.VM)
		owned  []vmKind.VM
		want   domain.ValidationErrors
	}{
		"what can be told from the request alone": {
			change: func(v *vmKind.VM) {
				v.Metadata.Name = "  "
				v.Spec.Flavor = "firecore"
				v.Spec.Ports = []port.Port{0}
				v.Spec.Network = vmKind.Network{Ingress: "open", Egress: "closed"}
				v.Metadata.Lifetime = -time.Second
			},
			want: domain.ValidationErrors{
				"name":             "required_field",
				"kind":             "invalid_kind",
				"ports":            "invalid_port",
				"network.ingress":  "invalid_access",
				"network.egress":   "invalid_access",
				"lifetime_seconds": "invalid_lifetime",
			},
		},
		"a vm of no flavor": {
			change: func(v *vmKind.VM) { v.Spec.Flavor = "" },
			want:   domain.ValidationErrors{"kind": "required_field"},
		},
		"a name nobody could show": {
			change: func(v *vmKind.VM) { v.Metadata.Name = strings.Repeat("a", vmKind.MaxNameLength+1) },
			want:   domain.ValidationErrors{"name": "invalid_name"},
		},
		"more ports than a vm may expose": {
			change: func(v *vmKind.VM) {
				v.Spec.Ports = make([]port.Port, vmKind.MaxPorts+1)
				for i := range v.Spec.Ports {
					v.Spec.Ports[i] = port.Port(1000 + i)
				}
			},
			want: domain.ValidationErrors{"ports": "too_many_ports"},
		},
		"a port there is not": {
			change: func(v *vmKind.VM) { v.Spec.Ports = []port.Port{65536} },
			want:   domain.ValidationErrors{"ports": "invalid_port"},
		},
		"more than one vm may be given": {
			change: func(v *vmKind.VM) {
				v.Spec.Resources = vmKind.Resources{CPUs: 5, Memory: 9 * vmtest.GiB, Disk: 51 * vmtest.GiB}
			},
			want: domain.ValidationErrors{
				"resources.cpus":   "too_large",
				"resources.memory": "too_large",
				"resources.disk":   "too_large",
			},
		},
		"nothing at all": {
			change: func(v *vmKind.VM) { v.Spec.Resources = vmKind.Resources{} },
			want: domain.ValidationErrors{
				"resources.cpus":   "required_field",
				"resources.memory": "required_field",
				"resources.disk":   "required_field",
			},
		},
		"less than a docker vm needs": {
			change: func(v *vmKind.VM) {
				v.Spec.Flavor = vmKind.FlavorDocker
				v.Spec.Resources = vmKind.Resources{CPUs: 1, Memory: 256 * vmtest.MiB, Disk: 2 * vmtest.GiB}
			},
			want: domain.ValidationErrors{
				"resources.memory": "too_small",
				"resources.disk":   "too_small",
			},
		},
		"longer than a vm may be kept": {
			change: func(v *vmKind.VM) { v.Metadata.Lifetime = 721 * time.Hour },
			want:   domain.ValidationErrors{"lifetime_seconds": "too_large"},
		},
		"more vms than one person may have": {
			owned: []vmKind.VM{
				owned("01", vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: vmtest.GiB}),
				owned("02", vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: vmtest.GiB}),
				owned("03", vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: vmtest.GiB}),
				owned("04", vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: vmtest.GiB}),
				owned("05", vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: vmtest.GiB}),
			},
			want: domain.ValidationErrors{"vms": "quota_exceeded"},
		},
		"more than one person's vms may be given between them": {
			owned: []vmKind.VM{
				owned("01", vmKind.Resources{CPUs: 4, Memory: 8 * vmtest.GiB, Disk: 50 * vmtest.GiB}),
				owned("02", vmKind.Resources{CPUs: 4, Memory: 7 * vmtest.GiB, Disk: 50 * vmtest.GiB}),
			},
			want: domain.ValidationErrors{
				"resources.cpus":   "quota_exceeded",
				"resources.memory": "quota_exceeded",
			},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithVMs(tt.owned...))

			_, invalid, err := w.VMs.Admit(ctx, asked(func(v *vmKind.VM) {
				if tt.change != nil {
					tt.change(v)
				}
			}))
			require.NoError(t, err)

			assert.Equal(t, tt.want, invalid)
		})
	}

	t.Run("one on its way out is not counted against its owner", func(t *testing.T) {
		t.Parallel()

		going := vmtest.In(owned("01", vmKind.Resources{CPUs: 4, Memory: 8 * vmtest.GiB, Disk: 50 * vmtest.GiB}), func(v *vmKind.VM) {
			v.Status.State = vmKind.Deleting
			v.Status.Expected = vmKind.Deleted
		})

		w := vmtest.New(vmtest.WithVMs(going, owned("02", vmKind.Resources{CPUs: 4, Memory: 7 * vmtest.GiB, Disk: 50 * vmtest.GiB})))

		_, invalid, err := w.VMs.Admit(ctx, asked())
		require.NoError(t, err)
		assert.Empty(t, invalid)
	})

	t.Run("a vm no node has room for is failed and given up on, on no node", func(t *testing.T) {
		t.Parallel()

		full := vmtest.Alive(vmtest.Node)
		full.Capacity.Allocated = vm.Resources{Memory: 63 * vmtest.GiB}

		admitted, invalid, err := vmtest.New(vmtest.WithNodes(full)).VMs.Admit(ctx, asked())
		require.NoError(t, err)
		require.Empty(t, invalid, "it is kept, failed: the dashboard shows why")

		assert.Equal(t, vmKind.Failed, admitted.Status.State)
		assert.Equal(t, vmKind.Failed, admitted.Status.Expected, "nothing keeps asking for it")
		assert.Equal(t, controlPlaneVMs.ReasonNoCapacity, admitted.Status.Reason)
		assert.Empty(t, admitted.Metadata.Node)
	})

	t.Run("a vm made from a snapshot is the snapshot's flavor and image, with room for its disk", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithSnapshots(vmtest.Snapshot("snapshot-uuid", "owner-uuid", func(s *snapshotKind.Snapshot) {
			s.Status.Flavor = vm.KindDocker
			s.Status.Image = "docker:28-dind"
			s.Status.Disk = 30 * vmtest.GiB
		})))

		admitted, invalid, err := w.VMs.Admit(ctx, asked(func(v *vmKind.VM) {
			v.Spec.Flavor = ""
			v.Spec.Source = &vmKind.Source{Snapshot: "snapshot-uuid"}
		}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, vmKind.FlavorDocker, admitted.Spec.Flavor)
		assert.Equal(t, "docker:28-dind", admitted.Spec.Image, "the image it was taken of")
		assert.Equal(t, uint64(30*vmtest.GiB), admitted.Spec.Resources.Disk)
		assert.Equal(t, &vmKind.Source{Snapshot: "snapshot-uuid"}, admitted.Spec.Source, "its node makes it from the snapshot")
		assert.Equal(t, "docker", admitted.Metadata.Labels[vmKind.LabelFlavor])
	})

	for name, tt := range map[string]struct {
		snapshot snapshotKind.Snapshot
		flavor   vmKind.Flavor
		want     domain.ValidationErrors
	}{
		"one that is not stored yet": {
			snapshot: vmtest.Snapshot("snapshot-uuid", "owner-uuid", func(s *snapshotKind.Snapshot) { s.Status.State = snapshotKind.Creating }),
			want:     domain.ValidationErrors{"snapshot_uuid": "snapshot_not_ready"},
		},
		"somebody else's": {
			snapshot: vmtest.Snapshot("snapshot-uuid", "other"),
			want:     domain.ValidationErrors{"snapshot_uuid": "not_found"},
		},
		"one of another flavor": {
			snapshot: vmtest.Snapshot("snapshot-uuid", "owner-uuid", func(s *snapshotKind.Snapshot) { s.Status.Flavor = vm.KindDocker }),
			flavor:   vmKind.FlavorMachine,
			want:     domain.ValidationErrors{"kind": "kind_mismatch"},
		},
	} {
		t.Run("not made from a snapshot that is "+name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithSnapshots(tt.snapshot))

			_, invalid, err := w.VMs.Admit(ctx, asked(func(v *vmKind.VM) {
				v.Spec.Flavor = tt.flavor
				v.Spec.Source = &vmKind.Source{Snapshot: "snapshot-uuid"}
			}))
			require.NoError(t, err)
			assert.Equal(t, tt.want, invalid)
		})
	}

	t.Run("it is given a slug no vm and no task holds", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		var taken string

		d := w.VMs.Dependencies
		d.Slugs = append([]slugs.Taken{func(_ context.Context, slug string) (bool, error) {
			if len(taken) == 0 {
				taken = slug

				return true, nil
			}

			return false, nil
		}}, d.Slugs...)

		admitted, _, err := controlPlaneVMs.New(d).Admit(ctx, asked())
		require.NoError(t, err)

		require.NotEmpty(t, taken)
		assert.NotEqual(t, taken, admitted.Metadata.Slug)
		assert.True(t, strings.HasPrefix(admitted.Metadata.Slug, "my-box-"))
	})

	t.Run("one a task holds is held", func(t *testing.T) {
		t.Parallel()

		run := vmtest.Run("run")
		w := vmtest.New(vmtest.WithTasks(run))

		held, err := w.VMs.Slugs[1](ctx, run.Metadata.Slug)
		require.NoError(t, err)
		assert.True(t, held, "a task's slug is a hostname the ingress serves")
	})
}

func TestVMs_Reconcile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := vmtest.New()

	in := func(state kind.State, expected kind.State, changes ...func(v *vmKind.VM)) vmKind.VM {
		return vmtest.In(vmtest.Running("01", "owner"), func(v *vmKind.VM) {
			v.Status.State = state
			v.Status.Expected = expected

			for _, change := range changes {
				change(v)
			}
		})
	}

	given := func(ports ...port.Port) func(v *vmKind.VM) {
		return func(v *vmKind.VM) {
			config := v.Spec.Config()
			config.Ports = ports
			v.Status.Applied = &config
		}
	}

	nothingApplied := func(v *vmKind.VM) { v.Status.Applied = nil }

	for name, tt := range map[string]struct {
		vm     vmKind.VM
		action string
	}{
		"one that runs while it was expected stopped is stopped": {vm: in(vmKind.Running, vmKind.Stopped), action: vmKind.ActionStop},
		"one admitted is made":                                             {vm: in(vmKind.Created, vmKind.Running, nothingApplied), action: vmKind.ActionCreate},
		"one stopped while it was expected running is started":             {vm: in(vmKind.Stopped, vmKind.Running), action: vmKind.ActionStart},
		"and so is one that failed":                                        {vm: in(vmKind.Failed, vmKind.Running), action: vmKind.ActionStart},
		"one whose node last gave it other ports is reconfigured":          {vm: in(vmKind.Running, vmKind.Running, given(22)), action: vmKind.ActionReconfigure},
		"even a stopped one":                                               {vm: in(vmKind.Stopped, vmKind.Stopped, given(22)), action: vmKind.ActionReconfigure},
		"a stop goes before a change":                                      {vm: in(vmKind.Running, vmKind.Stopped, given(22)), action: vmKind.ActionStop},
		"one doing what it is expected to is asked nothing":                {vm: in(vmKind.Running, vmKind.Running)},
		"nor is one stopped as it was asked":                               {vm: in(vmKind.Stopped, vmKind.Stopped)},
		"nor one given up on":                                              {vm: in(vmKind.Failed, vmKind.Failed)},
		"nor one failed that is to stay stopped":                           {vm: in(vmKind.Failed, vmKind.Stopped)},
		"nor one its node has applied nothing to":                          {vm: in(vmKind.Running, vmKind.Running, nothingApplied)},
		"nor one that failed with other ports, which a start gives it":     {vm: in(vmKind.Failed, vmKind.Failed, given(22))},
		"nor one on its way somewhere, which is the loop's to wait on":     {vm: in(vmKind.Starting, vmKind.Running, given(22))},
		"nor one expected deleted, which is the loop's to delete":          {vm: in(vmKind.Running, vmKind.Deleted)},
		"nor one made that is to stay stopped, which a start would create": {vm: in(vmKind.Created, vmKind.Stopped, nothingApplied)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			intents, err := w.VMs.Reconcile(ctx, tt.vm)
			require.NoError(t, err)

			if len(tt.action) == 0 {
				assert.Empty(t, intents)

				return
			}

			require.Len(t, intents, 1)
			assert.Equal(t, tt.action, intents[0].Action)
			assert.NotEmpty(t, intents[0].Reason)
			assert.True(t, vmKind.Descriptor().Allows(tt.action, tt.vm.Status.State), "what it asks for is allowed where it asks for it")
		})
	}
}

func TestVMs_Apply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	apply := func(w *vmtest.Workload, v vmKind.VM, update vmKind.UpdatePayload) (vmKind.VM, domain.ValidationErrors, error) {
		d := w.VMs.Dependencies
		d.Now = func() time.Time { return now }

		return controlPlaneVMs.New(d).Apply(ctx, v, vmKind.ActionUpdate, update)
	}

	t.Run("a new name and lifetime are the record's alone", func(t *testing.T) {
		t.Parallel()

		v := vmtest.Running("01", "owner")
		w := vmtest.New(vmtest.WithVMs(v))

		applied, invalid, err := apply(w, v, vmKind.UpdatePayload{Name: new("  renamed  "), Lifetime: new(time.Hour)})
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, "renamed", applied.Metadata.Name)
		assert.Equal(t, time.Hour, applied.Metadata.Lifetime)
		assert.Equal(t, now.Add(time.Hour), applied.Metadata.ExpiresAt, "counted from now")
		assert.Equal(t, v.Spec, applied.Spec, "nothing its node gave it changes")
	})

	t.Run("a lifetime of nothing keeps it until it is deleted", func(t *testing.T) {
		t.Parallel()

		v := vmtest.In(vmtest.Running("01", "owner"), func(v *vmKind.VM) {
			v.Metadata.Lifetime = time.Hour
			v.Metadata.ExpiresAt = now.Add(time.Hour)
		})

		applied, _, err := apply(vmtest.New(vmtest.WithVMs(v)), v, vmKind.UpdatePayload{Lifetime: new(time.Duration(0))})
		require.NoError(t, err)

		assert.Zero(t, applied.Metadata.Lifetime)
		assert.True(t, applied.Metadata.ExpiresAt.IsZero())
	})

	t.Run("new ports and a way of its network are its spec's, for its node to apply", func(t *testing.T) {
		t.Parallel()

		v := vmtest.Running("01", "owner")

		applied, invalid, err := apply(vmtest.New(vmtest.WithVMs(v)), v, vmKind.UpdatePayload{
			Ports:   &[]port.Port{443, 80, 443},
			Network: &vmKind.Network{Egress: vm.AccessDeny},
		})
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, []port.Port{80, 443}, applied.Spec.Ports)
		assert.Equal(t, vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, applied.Spec.Network, "the way it named changed, the other stayed")
		assert.Equal(t, vmKind.Running, applied.Status.State, "it is reconfigured once it is followed up on")
		assert.NotEqual(t, applied.Spec.Config(), *applied.Status.Applied, "which is what has it reconfigured")
	})

	t.Run("a vm that grows is held to what it may be given, and gets it", func(t *testing.T) {
		t.Parallel()

		v := vmtest.Running("01", "owner")
		grown := vmKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB}

		applied, invalid, err := apply(vmtest.New(vmtest.WithVMs(v)), v, vmKind.UpdatePayload{Resources: &grown})
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, grown, applied.Spec.Resources)
	})

	t.Run("one asked for what it has already changes nothing, wherever it is", func(t *testing.T) {
		t.Parallel()

		v := vmtest.In(vmtest.Running("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Starting })

		applied, invalid, err := apply(vmtest.New(vmtest.WithVMs(v)), v, vmKind.UpdatePayload{Ports: &[]port.Port{}, Resources: &v.Spec.Resources})
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, v.Spec, applied.Spec)
	})

	tooSmall := vmtest.Alive(vmtest.Node)
	tooSmall.Capacity.Memory = 2 * vmtest.GiB
	tooSmall.Capacity.Allocated = vm.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: 10 * vmtest.GiB}

	for name, tt := range map[string]struct {
		vm     vmKind.VM
		owned  []vmKind.VM
		nodes  []node.Node
		update vmKind.UpdatePayload
		want   domain.ValidationErrors
	}{
		"a disk does not shrink": {
			vm:     vmtest.Running("01", "owner"),
			update: vmKind.UpdatePayload{Resources: &vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: 5 * vmtest.GiB}},
			want:   domain.ValidationErrors{"resources.disk": "disk_cannot_shrink"},
		},
		"more than one vm may be given": {
			vm:     vmtest.Running("01", "owner"),
			update: vmKind.UpdatePayload{Resources: &vmKind.Resources{CPUs: 8, Memory: vmtest.GiB, Disk: 10 * vmtest.GiB}},
			want:   domain.ValidationErrors{"resources.cpus": "too_large"},
		},
		"more than its owner's vms may be given between them": {
			vm: vmtest.Running("01", "owner"),
			owned: []vmKind.VM{
				vmtest.In(vmtest.Running("02", "owner"), func(v *vmKind.VM) { v.Spec.Resources.Memory = 8 * vmtest.GiB }),
				vmtest.In(vmtest.Running("03", "owner"), func(v *vmKind.VM) { v.Spec.Resources.Memory = 7 * vmtest.GiB }),
			},
			update: vmKind.UpdatePayload{Resources: &vmKind.Resources{CPUs: 1, Memory: 2 * vmtest.GiB, Disk: 10 * vmtest.GiB}},
			want:   domain.ValidationErrors{"resources.memory": "quota_exceeded"},
		},
		"more than its node has room for": {
			vm:     vmtest.Running("01", "owner"),
			nodes:  []node.Node{tooSmall},
			update: vmKind.UpdatePayload{Resources: &vmKind.Resources{CPUs: 1, Memory: 4 * vmtest.GiB, Disk: 10 * vmtest.GiB}},
			want:   domain.ValidationErrors{"resources": "no_capacity"},
		},
		"a change to one on its way somewhere": {
			vm:     vmtest.In(vmtest.Running("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Starting }),
			update: vmKind.UpdatePayload{Ports: &[]port.Port{80}},
			want:   domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"a lifetime longer than a vm may be kept": {
			vm:     vmtest.Running("01", "owner"),
			update: vmKind.UpdatePayload{Lifetime: new(721 * time.Hour)},
			want:   domain.ValidationErrors{"lifetime_seconds": "too_large"},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			opts := []vmtest.Option{vmtest.WithVMs(append([]vmKind.VM{tt.vm}, tt.owned...)...)}
			if tt.nodes != nil {
				opts = append(opts, vmtest.WithNodes(tt.nodes...))
			}

			_, invalid, err := apply(vmtest.New(opts...), tt.vm, tt.update)
			require.NoError(t, err)
			assert.Equal(t, tt.want, invalid)
		})
	}

	t.Run("nothing else is done to a vm in the control plane", func(t *testing.T) {
		t.Parallel()

		_, _, err := vmtest.New().VMs.Apply(ctx, vmtest.Running("01", "owner"), vmKind.ActionStart, nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})
}

func TestVMs_Prepare(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ready := vmtest.Snapshot("snapshot-uuid", "owner")

	// changed is ready, as change leaves it.
	changed := func(change func(s *snapshotKind.Snapshot)) snapshotKind.Snapshot {
		return vmtest.Snapshot("snapshot-uuid", "owner", change)
	}

	t.Run("a restore from a snapshot of the vm's owner, flavor and engine, that its disk has room for, is sent as it is", func(t *testing.T) {
		t.Parallel()

		v := vmtest.Stopped("01", "owner")

		prepared, refused, err := vmtest.New(vmtest.WithVMs(v), vmtest.WithSnapshots(ready)).VMs.Prepare(ctx, v, vmKind.ActionRestore, vmKind.RestorePayload{SnapshotUUID: "snapshot-uuid"})
		require.NoError(t, err)
		assert.Empty(t, refused)
		assert.Equal(t, v, prepared)
	})

	for name, tt := range map[string]struct {
		snapshot snapshotKind.Snapshot
		nodes    []node.Node
		want     domain.ValidationErrors
	}{
		"one nobody has": {
			snapshot: vmtest.Snapshot("another", "owner"),
			want:     domain.ValidationErrors{"snapshot_uuid": "not_found"},
		},
		"somebody else's": {
			snapshot: changed(func(s *snapshotKind.Snapshot) { s.Metadata.OwnerUUID = "other" }),
			want:     domain.ValidationErrors{"snapshot_uuid": "not_found"},
		},
		"one still being taken": {
			snapshot: changed(func(s *snapshotKind.Snapshot) { s.Status.State = snapshotKind.Creating }),
			want:     domain.ValidationErrors{"snapshot_uuid": "snapshot_not_ready"},
		},
		"one of another flavor": {
			snapshot: changed(func(s *snapshotKind.Snapshot) { s.Status.Flavor = vm.KindDocker }),
			want:     domain.ValidationErrors{"snapshot_uuid": "kind_mismatch"},
		},
		"one larger than its disk": {
			snapshot: changed(func(s *snapshotKind.Snapshot) { s.Status.Disk = 11 * vmtest.GiB }),
			want:     domain.ValidationErrors{"snapshot_uuid": "disk_too_small"},
		},
		"one written by another engine": {
			snapshot: changed(func(s *snapshotKind.Snapshot) { s.Status.Engine = "firecracker/1.9" }),
			want:     domain.ValidationErrors{"snapshot_uuid": "engine_mismatch"},
		},
		"any, onto a vm whose node has gone quiet": {
			snapshot: ready,
			nodes:    []node.Node{vmtest.Gone(vmtest.Node)},
			want:     domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
	} {
		t.Run("no restore from "+name, func(t *testing.T) {
			t.Parallel()

			v := vmtest.Stopped("01", "owner")

			opts := []vmtest.Option{vmtest.WithVMs(v), vmtest.WithSnapshots(tt.snapshot)}
			if tt.nodes != nil {
				opts = append(opts, vmtest.WithNodes(tt.nodes...))
			}

			_, refused, err := vmtest.New(opts...).VMs.Prepare(ctx, v, vmKind.ActionRestore, vmKind.RestorePayload{SnapshotUUID: "snapshot-uuid"})
			require.NoError(t, err)
			assert.Equal(t, tt.want, refused)
		})
	}

	t.Run("a snapshot's engine is not held against a node that has not said its own", func(t *testing.T) {
		t.Parallel()

		quiet := vmtest.Alive(vmtest.Node)
		quiet.Capacity.Engine = ""

		v := vmtest.Stopped("01", "owner")
		taken := changed(func(s *snapshotKind.Snapshot) { s.Status.Engine = "firecracker/1.9" })

		_, refused, err := vmtest.New(vmtest.WithNodes(quiet), vmtest.WithVMs(v), vmtest.WithSnapshots(taken)).VMs.Prepare(ctx, v, vmKind.ActionRestore, vmKind.RestorePayload{SnapshotUUID: "snapshot-uuid"})
		require.NoError(t, err)
		assert.Empty(t, refused)
	})

	for _, action := range []string{vmKind.ActionStart, vmKind.ActionRestart, vmKind.ActionCreate} {
		t.Run("a vm on no node is placed before its "+action, func(t *testing.T) {
			t.Parallel()

			nowhere := vmtest.In(vmtest.Stopped("01", "owner"), func(v *vmKind.VM) {
				v.Metadata.Node = ""
				v.Status.State = vmKind.Failed
				v.Status.Reason = controlPlaneVMs.ReasonNoCapacity
			})

			prepared, refused, err := vmtest.New(vmtest.WithVMs(nowhere)).VMs.Prepare(ctx, nowhere, action, nil)
			require.NoError(t, err)
			require.Empty(t, refused)
			assert.Equal(t, vmtest.Node, prepared.Metadata.Node)
		})
	}

	t.Run("and refused while no node has room for it", func(t *testing.T) {
		t.Parallel()

		full := vmtest.Alive(vmtest.Node)
		full.Capacity.Allocated = vm.Resources{Memory: 64 * vmtest.GiB}

		nowhere := vmtest.In(vmtest.Stopped("01", "owner"), func(v *vmKind.VM) { v.Metadata.Node = "" })

		_, refused, err := vmtest.New(vmtest.WithNodes(full), vmtest.WithVMs(nowhere)).VMs.Prepare(ctx, nowhere, vmKind.ActionStart, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "no_capacity"}, refused)
	})

	t.Run("one on a node, and anything else, is sent as it is", func(t *testing.T) {
		t.Parallel()

		v := vmtest.Running("01", "owner")
		w := vmtest.New(vmtest.WithNodes(), vmtest.WithVMs(v))

		for _, action := range []string{vmKind.ActionStart, vmKind.ActionStop, vmKind.ActionDelete, vmKind.ActionReconfigure} {
			prepared, refused, err := w.VMs.Prepare(ctx, v, action, nil)
			require.NoError(t, err)
			assert.Empty(t, refused)
			assert.Equal(t, v, prepared, action)
		}
	})
}

func TestVMs_Extras(t *testing.T) {
	t.Parallel()

	w := vmtest.New()

	assert.Same(t, w.Runs, w.VMs.Extras(), "the code runner's runs are among anybody's vms")
}
