package container_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements/requirementstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// wanted is the container web, of nginx, in the Docker VM vmUUID names, of
// owner's, as changes say otherwise.
func wanted(vmUUID string, changes ...func(w *containerKind.Want)) containerKind.Want {
	w := containerKind.Want{
		OwnerUUID: "owner",
		VM:        stackKind.VMChoice{UUID: vmUUID},
		Spec:      containerKind.Spec{Name: "web", Image: "nginx:1.27"},
	}

	for _, change := range changes {
		change(&w)
	}

	return w
}

// unmade is a container its node has said nothing of docker having.
func unmade(c *containerKind.Container) {
	c.Status.Docker = nil
}

// stored is the container kept under uuid, and whether one is.
func stored(t *testing.T, w *requirementstest.Workload, uuid string) (containerKind.Container, bool) {
	t.Helper()

	r, kept := w.Kept(containerKind.Name, uuid)
	if !kept {
		return containerKind.Container{}, false
	}

	return typed(t, r.Raw), true
}

func TestGraph_Requires(t *testing.T) {
	t.Parallel()

	containers := requirementstest.New().Kinds[containerKind.Name]

	t.Run("its vm, each network it joins and each volume it mounts by its name, once", func(t *testing.T) {
		t.Parallel()

		w := containerKind.Want{
			OwnerUUID: "owner",
			VM:        stackKind.VMChoice{UUID: "vm-1", Name: "builds", Ports: []port.Port{}},
			Spec: containerKind.Spec{
				Name:     "web",
				Image:    "nginx:1.27",
				Networks: []string{"backend", "bridge", "frontend", "host", "none", "backend"},
				Mounts: []containerKind.Mount{
					{Type: containerKind.MountVolume, Source: "data", Target: "/data"},
					{Type: containerKind.MountBind, Source: "/srv", Target: "/srv"},
					{Type: containerKind.MountTmpfs, Target: "/tmp"},
					{Type: containerKind.MountVolume, Target: "/scratch"},
					{Type: containerKind.MountVolume, Source: "data", Target: "/backup", ReadOnly: true},
					{Type: containerKind.MountVolume, Source: "cache", Target: "/cache"},
				},
				External: []string{"network/frontend", "volume/cache"},
			},
		}

		required, err := containers.Requires(w)
		require.NoError(t, err)

		assert.Equal(t, []graph.Requirement{
			{Kind: vmKind.Name, Name: "vm-1", Spec: vmKind.Want{OwnerUUID: "owner", VM: w.VM}},
			{Kind: networkKind.Name, Name: "backend", Spec: networkKind.Want{OwnerUUID: "owner", VM: w.VM, Spec: networkKind.Spec{Name: "backend"}}},
			{Kind: networkKind.Name, Name: "frontend", Spec: networkKind.Want{OwnerUUID: "owner", VM: w.VM, Spec: networkKind.Spec{Name: "frontend"}}, External: true},
			{Kind: volumeKind.Name, Name: "data", Spec: volumeKind.Want{OwnerUUID: "owner", VM: w.VM, Spec: volumeKind.Spec{Name: "data"}}},
			{Kind: volumeKind.Name, Name: "cache", Spec: volumeKind.Want{OwnerUUID: "owner", VM: w.VM, Spec: volumeKind.Spec{Name: "cache"}}, External: true},
		}, required, "bridge, host and none are every dockerd's own, a bind, a tmpfs and a volume docker names are none, and what is not its own is external")
	})

	t.Run("its vm is wanted as it wants it, so that whichever requirement of it a graph keeps can make it", func(t *testing.T) {
		t.Parallel()

		w := wanted("vm-1", func(w *containerKind.Want) {
			w.VM = stackKind.VMChoice{UUID: "vm-1", Name: "builds", Resources: &stackKind.Resources{Memory: 4 << 30}}
		})

		required, err := containers.Requires(&w)
		require.NoError(t, err, "a want is handed over as one, or as a pointer to one")
		require.Len(t, required, 1)

		assert.Equal(t, graph.ID(vmKind.Name, "vm-1"), required[0].ID())
		assert.Equal(t, vmKind.Want{OwnerUUID: "owner", VM: w.VM}, required[0].Spec)
	})

	for name, spec := range map[string]any{
		"what is not a container's want": networkKind.Want{VM: stackKind.VMChoice{UUID: "vm-1"}, Spec: networkKind.Spec{Name: "backend"}},
		"a want in no vm":                wanted(""),
		"nothing":                        nil,
		"a want that is not there":       (*containerKind.Want)(nil),
	} {
		t.Run("not "+name, func(t *testing.T) {
			t.Parallel()

			_, err := containers.Requires(spec)
			assert.ErrorIs(t, err, kind.ErrInvalidPayload)
		})
	}
}

func TestGraph_Root(t *testing.T) {
	t.Parallel()

	containers := requirementstest.New().Kinds[containerKind.Name]

	roots, isRoot := containers.(graph.RootKind)
	require.True(t, isRoot, "a container is the root of a graph of its own")

	t.Run("one no stack owns is its record's uuid, its owner's, labelled as its record is and asked for as its spec asks, in the vm its spec names", func(t *testing.T) {
		t.Parallel()

		r := blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
			c.Metadata.Labels = map[string]string{"team": "shop"}
			c.Spec.Networks = []string{"backend"}
			c.Spec.Mounts = []containerKind.Mount{{Type: containerKind.MountVolume, Source: "data", Target: "/data"}}
			c.Spec.External = []string{"volume/data"}
		})

		root, err := roots.Root(r)
		require.NoError(t, err)

		assert.Equal(t, graph.Requirement{
			Kind: containerKind.Name,
			Name: "web",
			Spec: containerKind.Want{
				UUID:      "c-1",
				OwnerUUID: "owner",
				Labels:    map[string]string{"team": "shop"},
				VM:        stackKind.VMChoice{UUID: "vm-1"},
				Spec:      typed(t, r).Spec,
			},
		}, root)

		// what it needs is what it needs as a root: its vm, its network
		// and the volume that is not its own.
		required, err := containers.Requires(root.Spec)
		require.NoError(t, err)

		var ids []graph.NodeID
		for _, each := range required {
			ids = append(ids, each.ID())
		}

		assert.Equal(t, []graph.NodeID{graph.ID(vmKind.Name, "vm-1"), graph.ID(networkKind.Name, "backend"), graph.ID(volumeKind.Name, "data")}, ids)
		assert.True(t, required[2].External)
	})

	t.Run("its vm is all its spec says of it, so that it is made again as it was asked for", func(t *testing.T) {
		t.Parallel()

		described := stackKind.VMChoice{UUID: "vm-1", Name: "builds", Resources: &stackKind.Resources{CPUs: 2, Memory: 4 << 30}, Ports: []port.Port{}}

		root, err := roots.Root(blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
			c.Spec.VM = described
		}))
		require.NoError(t, err)

		w, ok := root.Spec.(containerKind.Want)
		require.True(t, ok)
		assert.Equal(t, described, w.VM)

		required, err := containers.Requires(root.Spec)
		require.NoError(t, err)
		assert.Equal(t, vmKind.Want{OwnerUUID: "owner", VM: described}, required[0].Spec, "which its vm is wanted as")
	})

	t.Run("one docker names is named by its record's uuid", func(t *testing.T) {
		t.Parallel()

		root, err := roots.Root(blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
			c.Spec.Name = ""
		}))
		require.NoError(t, err)

		assert.Equal(t, graph.ID(containerKind.Name, "c-1"), root.ID())
	})

	for name, tt := range map[string]struct {
		record kind.Raw
		want   error
	}{
		"a record of another kind": {
			record: blockstest.A(networkKind.Name, "n-1", "vm-1", networkKind.Spec{Name: "backend"}, networkKind.Status{}),
			want:   kind.ErrUnknownKind,
		},
		"a container in no vm": {
			record: blockstest.AContainer("c-1", "", containerKind.Running, containerKind.Running),
			want:   kind.ErrInvalidPayload,
		},
	} {
		t.Run("not "+name, func(t *testing.T) {
			t.Parallel()

			_, err := roots.Root(tt.record)
			assert.ErrorIs(t, err, tt.want)
		})
	}

	t.Run("nor one whose spec cannot be read", func(t *testing.T) {
		t.Parallel()

		r := blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running)
		r.Spec = []byte(`{"vm":`)

		_, err := roots.Root(r)
		assert.Error(t, err)
	})
}

func TestGraph_Exists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// whether it is made, and its record, made or not: none when there is no
	// record of it.
	for name, tt := range map[string]struct {
		container kind.Raw
		exists    bool
		none      bool
	}{
		"one that runs":                                 {container: blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running), exists: true},
		"one stopped":                                   {container: blockstest.AContainer("c-1", "vm-1", containerKind.Stopped, containerKind.Stopped), exists: true},
		"one that ran to its end":                       {container: blockstest.AContainer("c-1", "vm-1", containerKind.Completed, containerKind.Running), exists: true},
		"one being started":                             {container: blockstest.AContainer("c-1", "vm-1", containerKind.Starting, containerKind.Running), exists: true},
		"one that failed once docker had it":            {container: blockstest.AContainer("c-1", "vm-1", containerKind.Failed, containerKind.Running), exists: true},
		"not one that failed before docker ever had it": {container: blockstest.AContainer("c-1", "vm-1", containerKind.Failed, containerKind.Running, unmade)},
		"nor one not made yet":                          {container: blockstest.AContainer("c-1", "vm-1", containerKind.Pending, containerKind.Running, unmade)},
		"nor one being made":                            {container: blockstest.AContainer("c-1", "vm-1", containerKind.Creating, containerKind.Running, unmade)},
		"nor one its vm has none of":                    {container: blockstest.AContainer("c-1", "vm-1", containerKind.Missing, containerKind.Running)},
		"nor one waiting on its vm":                     {container: blockstest.AContainer("c-1", "vm-1", containerKind.Waiting, containerKind.Running)},
		"nor one being removed":                         {container: blockstest.AContainer("c-1", "vm-1", containerKind.Removing, kind.Deleted)},
		"nor one that runs and is to be removed":        {container: blockstest.AContainer("c-1", "vm-1", containerKind.Running, kind.Deleted)},
		"nor one of another name":                       {container: blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) { c.Spec.Name = "db" }), none: true},
		"nor one of its name in another vm":             {container: blockstest.AContainer("c-1", "vm-2", containerKind.Running, containerKind.Running), none: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))
			w.Keep(tt.container)

			ref, exists, err := w.Kinds[containerKind.Name].Exists(ctx, wanted("vm-1"))
			require.NoError(t, err)

			assert.Equal(t, tt.exists, exists)

			if tt.none {
				assert.Zero(t, ref, "there is no record of it")
			} else {
				assert.Equal(t, graph.Ref{Kind: containerKind.Name, UUID: "c-1"}, ref, "its record, made or not")
			}
		})
	}

	t.Run("nor one there is no record of, which is no record", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		ref, exists, err := w.Kinds[containerKind.Name].Exists(ctx, wanted("vm-1"))
		require.NoError(t, err)
		assert.False(t, exists)
		assert.Zero(t, ref)
	})

	t.Run("one known by its uuid is found by it, whatever it is called", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) { c.Spec.Name = "" }))

		ref, exists, err := w.Kinds[containerKind.Name].Exists(ctx, wanted("vm-1", func(w *containerKind.Want) {
			w.UUID = "c-1"
			w.Spec.Name = ""
		}))
		require.NoError(t, err)

		assert.True(t, exists)
		assert.Equal(t, "c-1", ref.UUID)
	})

	t.Run("of two of one name, the one that is not on its way out", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-old", "vm-1", containerKind.Removing, kind.Deleted))
		w.Keep(blockstest.AContainer("c-new", "vm-1", containerKind.Running, containerKind.Running))

		ref, exists, err := w.Kinds[containerKind.Name].Exists(ctx, wanted("vm-1"))
		require.NoError(t, err)

		assert.True(t, exists)
		assert.Equal(t, "c-new", ref.UUID)
	})

	t.Run("and one on its way out, not made, when it is the only one", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-old", "vm-1", containerKind.Removing, kind.Deleted))

		ref, exists, err := w.Kinds[containerKind.Name].Exists(ctx, wanted("vm-1"))
		require.NoError(t, err)

		assert.False(t, exists)
		assert.Equal(t, "c-old", ref.UUID)
	})
}

func TestGraph_Create(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inVM := map[graph.NodeID]graph.Ref{graph.ID(vmKind.Name, "vm-1"): {Kind: vmKind.Name, UUID: "vm-1"}}

	t.Run("one there is no record of is admitted into its vm, made there by its node, and is its record", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		ref, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1", func(w *containerKind.Want) {
			w.Labels = map[string]string{"stack": "shop"}
			w.VM.Name = "builds"
		}), inVM)
		require.NoError(t, err)

		c, kept := stored(t, w, ref.UUID)
		require.True(t, kept)

		assert.Equal(t, containerKind.Name, ref.Kind)
		assert.Equal(t, containerKind.Running, c.Status.Status.State, "as its node made it")
		assert.Equal(t, "owner", c.Metadata.OwnerUUID)
		assert.Equal(t, map[string]string{"stack": "shop"}, c.Metadata.Labels)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, c.Metadata.Refs, "it lives in its vm")
		assert.Empty(t, c.Metadata.Owners, "and nothing owns it until its graph says so")
		assert.Equal(t, int64(1), c.Metadata.Generation)
		assert.Equal(t, vmKind.ChoiceOf(vmtest.Docker("vm-1", "owner")), c.Spec.VM, "its vm as its record has it, as a container is kept")
		assert.Equal(t, "web", c.Spec.Name)

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, containerKind.ActionCreate, sent[0].Action)
		assert.Equal(t, ref.UUID, sent[0].UUID)
	})

	t.Run("one something owns from its admission on, a stack's, is admitted owned by it, and never taken for a root", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		ref, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1", func(w *containerKind.Want) {
			w.Spec.Name = "shop_web"
			w.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}
			w.Labels = map[string]string{stackKind.LabelStack: "stack-uuid", stackKind.LabelService: "web"}
		}), inVM)
		require.NoError(t, err)

		c, kept := stored(t, w, ref.UUID)
		require.True(t, kept)

		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}, c.Metadata.Owners)
		assert.Equal(t, map[string]string{stackKind.LabelStack: "stack-uuid", stackKind.LabelService: "web"}, c.Metadata.Labels)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, c.Metadata.Refs, "and lives in its vm")
	})

	t.Run("one to be made stopped, a service of a stack expected stopped, is expected stopped before its create is asked, which then leaves it so", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		ref, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1", func(w *containerKind.Want) {
			w.Spec.Name = "shop_web"
			w.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}
			w.Stopped = true
		}), inVM)
		require.NoError(t, err)

		c, kept := stored(t, w, ref.UUID)
		require.True(t, kept)
		assert.Equal(t, containerKind.Stopped, c.Status.Expected)

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, containerKind.ActionCreate, sent[0].Action)

		asked := typed(t, sent[0].Resource)
		assert.Equal(t, containerKind.Stopped, asked.Status.Expected, "its node is asked to make it stopped, which it makes it and does not start it")
	})

	t.Run("and so is one gone missing, made again", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Missing, containerKind.Running, func(c *containerKind.Container) {
			c.Spec.Name = "web"
			c.Metadata.Name = "web"
		}))

		_, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1", func(w *containerKind.Want) { w.Stopped = true }), inVM)
		require.NoError(t, err)

		c, _ := stored(t, w, "c-1")
		assert.Equal(t, containerKind.Stopped, c.Status.Expected)

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, containerKind.Stopped, typed(t, sent[0].Resource).Status.Expected)
	})

	t.Run("under its record's uuid, when it is known by one, as its admission kept it", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-held", "vm-1", containerKind.Pending, containerKind.Running, unmade))

		ref, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1", func(w *containerKind.Want) { w.UUID = "c-held" }), inVM)
		require.NoError(t, err)

		assert.Equal(t, graph.Ref{Kind: containerKind.Name, UUID: "c-held"}, ref)

		c, kept := stored(t, w, "c-held")
		require.True(t, kept)
		assert.Equal(t, containerKind.Running, c.Status.Status.State)
	})

	t.Run("but one whose record is gone, a root, is never admitted again under its uuid", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		_, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1", func(w *containerKind.Want) { w.UUID = "c-gone" }), inVM)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is gone")

		_, kept := stored(t, w, "c-gone")
		assert.False(t, kept, "nothing is kept under its uuid")
		assert.Empty(t, w.Node.Sent(), "nor asked of its node")
	})

	t.Run("one made already is its record, and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running))

		ref, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1"), inVM)
		require.NoError(t, err)

		assert.Equal(t, graph.Ref{Kind: containerKind.Name, UUID: "c-1"}, ref)
		assert.Empty(t, w.Node.Sent())
	})

	for name, tt := range map[string]struct {
		container kind.Raw
		asked     string
	}{
		"one its vm has none of is asked to be made again": {
			container: blockstest.AContainer("c-1", "vm-1", containerKind.Missing, containerKind.Running),
			asked:     containerKind.ActionCreate,
		},
		"one not asked to be made yet is asked": {
			container: blockstest.AContainer("c-1", "vm-1", containerKind.Pending, containerKind.Running, unmade),
			asked:     containerKind.ActionCreate,
		},
		"one that failed before docker ever had it is started, which makes it": {
			container: blockstest.AContainer("c-1", "vm-1", containerKind.Failed, containerKind.Running, unmade),
			asked:     containerKind.ActionStart,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
			w.Keep(tt.container)

			ref, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1"), inVM)
			require.NoError(t, err)

			assert.Equal(t, graph.Ref{Kind: containerKind.Name, UUID: "c-1"}, ref, "under the record it has")

			sent := w.Node.Sent()
			require.Len(t, sent, 1)
			assert.Equal(t, tt.asked, sent[0].Action)

			c, _ := stored(t, w, "c-1")
			assert.Equal(t, containerKind.Running, c.Status.Status.State)
		})
	}

	for name, node := range map[string]string{
		"one on no node, admitted before its vm was made, or kept when its vm went, is placed on its vm's node first, and made there": "",
		"and so is one on another node, its vm made again elsewhere":                                                                  "workload-orchestrator-02",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
			w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Missing, containerKind.Running, unmade, func(c *containerKind.Container) {
				c.Metadata.Node = node
			}))

			ref, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1"), inVM)
			require.NoError(t, err)
			assert.Equal(t, "c-1", ref.UUID)

			sent := w.Node.Sent()
			require.Len(t, sent, 1)
			assert.Equal(t, containerKind.ActionCreate, sent[0].Action)
			assert.Equal(t, vmtest.Node, sent[0].Node, "sent where its vm is")

			c, _ := stored(t, w, "c-1")
			assert.Equal(t, vmtest.Node, c.Metadata.Node)
			assert.Equal(t, containerKind.Running, c.Status.Status.State)
		})
	}

	t.Run("one being made is waited on, and nothing is asked again", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.KeepWaiting(blockstest.AContainer("c-1", "vm-1", containerKind.Creating, containerKind.Running, unmade), containerKind.ActionCreate, "create-1")

		waiting, _ := w.Kept(containerKind.Name, "c-1")
		answered, _ := requirementstest.Carry(requirementstest.CommandOf(waiting))

		said := make(chan error, 1)
		go func() {
			time.Sleep(20 * time.Millisecond)
			said <- w.Node.Say(ctx, answered)
		}()

		ref, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1"), inVM)
		require.NoError(t, err)
		require.NoError(t, <-said)

		assert.Equal(t, graph.Ref{Kind: containerKind.Name, UUID: "c-1"}, ref)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("and one its node says nothing of in time is not made, saying what it is", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Requirements.Patience = 20 * time.Millisecond
		w.KeepWaiting(blockstest.AContainer("c-1", "vm-1", containerKind.Creating, containerKind.Running, unmade), containerKind.ActionCreate, "create-1")

		_, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1"), inVM)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not made: it is creating")
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one its node could not make is not made, saying why, and is kept failed", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Pending, containerKind.Running, unmade))
		w.Node.Answer(func(command kind.ActOnResource) (kind.ResourceActedOn, bool) {
			return requirementstest.Fail(command, "pull access denied for nginx"), true
		})

		_, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1", func(w *containerKind.Want) { w.UUID = "c-1" }), inVM)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "pull access denied for nginx")

		c, kept := stored(t, w, "c-1")
		require.True(t, kept)
		assert.Equal(t, containerKind.Failed, c.Status.Status.State)
	})

	for name, tt := range map[string]struct {
		vm     vmKind.VM
		reason string
	}{
		"nothing is asked while its vm does not run, which it waits on": {
			vm:     vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped }),
			reason: "its vm is stopped",
		},
		"nor while its vm comes up": {
			vm:     vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Starting }),
			reason: "its vm is starting",
		},
		"nor while its vm is being deleted": {
			vm:     vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.Expected = vmKind.Deleted }),
			reason: "its vm is being deleted",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(tt.vm))

			_, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1"), inVM)

			var waiting *graph.Waiting
			require.ErrorAs(t, err, &waiting)
			assert.Equal(t, tt.reason, waiting.Reason)

			assert.Empty(t, w.Node.Sent())
			assert.Zero(t, w.Memory.Len(containerKind.Name), "and nothing is kept")
		})
	}

	t.Run("one on its way out is waited on", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Removing, kind.Deleted))

		_, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1"), inVM)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
		assert.Equal(t, "it is being removed", waiting.Reason)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("a vm that is not there is not waited on", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New()

		_, err := w.Kinds[containerKind.Name].Create(ctx, wanted("vm-1"), inVM)

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.NotErrorAs(t, err, new(*graph.Waiting), "a graph makes its vm first")
	})

	t.Run("nor is what is not a container's want", func(t *testing.T) {
		t.Parallel()

		_, err := requirementstest.New().Kinds[containerKind.Name].Create(ctx, vmKind.Want{VM: stackKind.VMChoice{UUID: "vm-1"}}, inVM)
		assert.ErrorIs(t, err, kind.ErrInvalidPayload)
	})
}

func TestGraph_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	web := graph.Ref{Kind: containerKind.Name, UUID: "c-1"}

	t.Run("one is removed by force, running or not, and is gone once its node removed it", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running))

		require.NoError(t, w.Kinds[containerKind.Name].Delete(ctx, web))

		_, kept := stored(t, w, "c-1")
		assert.False(t, kept)

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, containerKind.ActionDelete, sent[0].Action)
		assert.JSONEq(t, `{"force": true}`, string(sent[0].Payload))
	})

	t.Run("one gone already is removed, and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		require.NoError(t, w.Kinds[containerKind.Name].Delete(ctx, web))
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one being removed is waited on until it is gone, and nothing is asked again", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.KeepWaiting(blockstest.AContainer("c-1", "vm-1", containerKind.Removing, kind.Deleted), containerKind.ActionDelete, "delete-1")

		removing, _ := w.Kept(containerKind.Name, "c-1")
		answered, _ := requirementstest.Carry(requirementstest.CommandOf(removing))

		said := make(chan error, 1)
		go func() {
			time.Sleep(20 * time.Millisecond)
			said <- w.Node.Say(ctx, answered)
		}()

		require.NoError(t, w.Kinds[containerKind.Name].Delete(ctx, web))
		require.NoError(t, <-said)

		_, kept := stored(t, w, "c-1")
		assert.False(t, kept)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("nothing is asked while its vm does not run, which it waits on", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Waiting, containerKind.Running))

		err := w.Kinds[containerKind.Name].Delete(ctx, web)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
		assert.Equal(t, "its vm is stopped", waiting.Reason)

		_, kept := stored(t, w, "c-1")
		assert.True(t, kept)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("but one on no node, whose vm is still being made, goes at once, and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Scheduled })))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Pending, containerKind.Running, unmade, func(c *containerKind.Container) {
			c.Metadata.Node = ""
		}))

		require.NoError(t, w.Kinds[containerKind.Name].Delete(ctx, web))

		_, kept := stored(t, w, "c-1")
		assert.False(t, kept)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one its node would not remove is not removed, saying why", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running))
		w.Node.Answer(func(command kind.ActOnResource) (kind.ResourceActedOn, bool) {
			return requirementstest.Fail(command, "device or resource busy"), true
		})

		err := w.Kinds[containerKind.Name].Delete(ctx, web)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not removed: it is failed: device or resource busy")

		_, kept := stored(t, w, "c-1")
		assert.True(t, kept)
	})

	t.Run("what is not a container is not a container's to remove", func(t *testing.T) {
		t.Parallel()

		err := requirementstest.New().Kinds[containerKind.Name].Delete(ctx, graph.Ref{Kind: networkKind.Name, UUID: "c-1"})
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})
}
