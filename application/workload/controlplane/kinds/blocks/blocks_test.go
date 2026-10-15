package blocks_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	controlPlaneContainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/container"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// seen is a container as a node reports it in vm-1: the resource uuid names,
// when the platform made it, or nobody's when uuid is empty.
func seen(t *testing.T, uuid string, docker containerKind.Docker, owners ...kind.Reference) kind.Observation {
	t.Helper()

	status, err := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.StateOf(docker.State, docker.RestartPolicy)}, Docker: &docker})
	require.NoError(t, err)

	return kind.Observation{
		Kind:   containerKind.Name,
		UUID:   uuid,
		Owners: append([]kind.Reference{{Kind: "vm", UUID: "vm-1"}}, owners...),
		Status: status,
	}
}

// witnessed has the containers' strategy hear of each instance, nobody's
// record, in a heartbeat of its own taken at a moment.
func witnessed(t *testing.T, w *blockstest.Workload, at time.Time, instances ...kind.Observation) {
	t.Helper()

	for _, instance := range instances {
		require.NoError(t, w.Containers.Witnessed(context.Background(), vmtest.Node, instance, false, at))
	}
}

// beating has the containers' strategy hear of each instance, nobody's record,
// in a heartbeat of its own every second its node beats from one moment to
// another, both among them.
func beating(t *testing.T, w *blockstest.Workload, from time.Time, to time.Time, instances ...kind.Observation) {
	t.Helper()

	for at := from; !at.After(to); at = at.Add(time.Second) {
		witnessed(t, w, at, instances...)
	}
}

func TestBlocks_Witnessed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	at := time.Now()

	t.Run("what nobody keeps a record of is shown as nobody's, its vm's owner's", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("kept", "vm-1", containerKind.Running, containerKind.Running))

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, seen(t, "kept", containerKind.Docker{ID: "c-kept", Name: "web", State: "running"}), true, at))

		witnessed(t, w, at,
			seen(t, "", containerKind.Docker{ID: "c-db", Name: "db", State: "running", CreatedAt: at.Add(-time.Hour)}),
			seen(t, "", containerKind.Docker{ID: "c-shop", Name: "shop_web", State: "running", Labels: map[string]string{"workload.stack": "stack-uuid", "workload.stack.service": "web"}, CreatedAt: at}),
		)

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 2, "what a record speaks for is the record's")

		shop, db := extras[0], extras[1]

		assert.Equal(t, "shop_web", shop.Metadata.Name, "newest first")
		assert.Equal(t, blockKinds.ManagedByNobody, shop.Metadata.Labels[blockKinds.LabelManagedBy], "a stack's label alone keeps nothing: what a stack's graph made is a record")
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, shop.Metadata.Owners)

		assert.Equal(t, blockKinds.ManagedByNobody, db.Metadata.Labels[blockKinds.LabelManagedBy], "made from the vm's terminal: unmanaged")
		assert.Equal(t, "owner", db.Metadata.OwnerUUID, "its vm's owner's")
		assert.Equal(t, vmtest.Node, db.Metadata.Node)
		assert.Equal(t, blockKinds.Derived(containerKind.Name, "vm-1", "c-db"), db.Metadata.UUID, "known by its docker id")

		one, err := w.Containers.One(ctx, db.Metadata.UUID)
		require.NoError(t, err)
		assert.Equal(t, db, one)

		_, err = w.Containers.One(ctx, "kept")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a record is not an extra")
	})

	t.Run("what was seen of one before a record was kept of it is let go of once the record is heard of", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		web := seen(t, "web-uuid", containerKind.Docker{ID: "c-web", Name: "web", State: "running", Labels: labelled(t, "web-uuid")})

		beating(t, w, at, at.Add(blockstest.SureAfter), web)

		_, err := w.Containers.One(ctx, "web-uuid")
		require.NoError(t, err, "nobody's, while nobody keeps a record of it")

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, web, true, at.Add(blockstest.SureAfter+time.Second)))

		_, err = w.Containers.One(ctx, "web-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a record speaks for it now, and it is not shown twice")
	})

	t.Run("what was seen is shown as it was last seen, until it goes unheard for long enough", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))

		inVM2 := seen(t, "", containerKind.Docker{ID: "c-2", Name: "cache", State: "running"})
		inVM2.Owners = []kind.Reference{{Kind: "vm", UUID: "vm-2"}}

		witnessed(t, w, at, seen(t, "", containerKind.Docker{ID: "c-1", Name: "db", State: "running"}))
		witnessed(t, w, at.Add(-2*time.Minute), inVM2)

		// what was seen a moment before, heard after it.
		witnessed(t, w, at.Add(-time.Second), seen(t, "", containerKind.Docker{ID: "c-1", Name: "db", State: "exited"}))

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 1, "what nothing has said for long enough, a vm's stopped or gone, is let go of")
		assert.Equal(t, "db", extras[0].Metadata.Name)
		assert.Equal(t, "running", blocks.ObservedOf(extras[0].Status).Docker.State, "as it was last seen, not as it was seen before that")

		assert.Empty(t, w.Sightings.In(containerKind.Name, "vm-2"))
	})

	t.Run("what is labelled as the platform's with no record is not taken in, nor shown at all from the heartbeats its node took before its record went", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		web := seen(t, "deleted-a-moment-ago", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "deleted-a-moment-ago")})

		// the heartbeats on their way as its delete's result was heard.
		for _, beat := range []time.Duration{-time.Second, -500 * time.Millisecond, 0} {
			witnessed(t, w, at.Add(beat), web)
		}

		_, kept := w.Kept(containerKind.Name, "deleted-a-moment-ago")
		assert.False(t, kept, "what a node saw before it removed it does not come back")

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, extras, "nor is it shown, as nobody's")

		_, err = w.Containers.One(ctx, "deleted-a-moment-ago")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = w.Containers.Resolve(ctx, kind.Reference{Kind: "vm", UUID: "vm-1"}, "web")
		assert.ErrorIs(t, err, domain.ErrNotExists, "nor named, to be acted on")
	})

	t.Run("nor from a backlog of them as long as a record may go unheard, which what else its node says of its vm, heard in time, leaves behind", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		web := seen(t, "deleted-a-moment-ago", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "deleted-a-moment-ago")})

		status, err := json.Marshal(networkKind.Status{Status: kind.Status{State: networkKind.Present}, Docker: &networkKind.Docker{ID: "n-bridge", Name: "bridge", Driver: "bridge"}})
		require.NoError(t, err)

		bridge := kind.Observation{Kind: networkKind.Name, Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, Status: status}

		// web's heartbeats are heard 35 s after its node took them, and its
		// vm's networks' as they are said: each beat of web its node took
		// over the 35 s before it removed web is heard with the networks'
		// of 35 s later, once web's record went.
		for after := time.Duration(0); after < 35*time.Second; after += time.Second {
			require.NoError(t, w.Networks.Witnessed(ctx, vmtest.Node, bridge, false, at.Add(after+35*time.Second)))
			witnessed(t, w, at.Add(after), web)
		}

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, extras)

		_, err = w.Containers.One(ctx, "deleted-a-moment-ago")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("but one its node goes on saying is shown as nobody's once its node has said it for as long as a record may go unheard", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		web := seen(t, "outlived-its-record", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "outlived-its-record")})

		beating(t, w, at, at.Add(blockstest.SureAfter-time.Second), web)

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, extras, "not before")

		witnessed(t, w, at.Add(blockstest.SureAfter), web)

		extras, err = w.Containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 1)
		assert.Equal(t, "outlived-its-record", extras[0].Metadata.UUID)
		assert.Equal(t, blockKinds.ManagedByNobody, extras[0].Metadata.Labels[blockKinds.LabelManagedBy])

		_, kept := w.Kept(containerKind.Name, "outlived-its-record")
		assert.False(t, kept, "and is not taken in")
	})

	t.Run("but in the first look into a vm whose disk was just restored, it is taken in again, as it was asked for", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
		w := blockstest.New(vmtest.WithVMs(restored))

		first := seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "from-the-snapshot")})
		second := seen(t, "also-from-the-snapshot", containerKind.Docker{ID: "c2", Name: "api", State: "running", Labels: labelled(t, "also-from-the-snapshot")})

		witnessed(t, w, at, first, second)

		adopted, kept := w.Kept(containerKind.Name, "from-the-snapshot")
		require.True(t, kept)

		c, err := kind.Decode[containerKind.Spec, containerKind.Status](adopted.Raw)
		require.NoError(t, err)

		assert.Equal(t, "owner", c.Metadata.OwnerUUID)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, c.Metadata.Refs, "it lives in its vm")
		assert.Empty(t, c.Metadata.Owners, "which does not own it")
		assert.Equal(t, int64(1), c.Metadata.Generation, "its spec its first")
		assert.Equal(t, vmtest.Node, c.Metadata.Node)
		assert.Equal(t, "nginx:1.27", c.Spec.Image, "as its label says it was asked for")
		assert.Equal(t, containerKind.Running, c.Status.Status.State)
		assert.Equal(t, containerKind.Running, c.Status.Expected)

		_, kept = w.Kept(containerKind.Name, "also-from-the-snapshot")
		assert.True(t, kept, "every heartbeat of that beat is that look")

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, extras, "and it is not shown as nobody's")

		require.NoError(t, w.Resources.Delete(ctx, containerKind.Name, "from-the-snapshot"))
		witnessed(t, w, at.Add(time.Second), first)

		_, kept = w.Kept(containerKind.Name, "from-the-snapshot")
		assert.False(t, kept, "only the first look after the restore takes anything in")
	})

	t.Run("and one its labels say is a stack's, of a stack that is there, is taken in as that stack's, as its kind says", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
		w := blockstest.New(vmtest.WithVMs(restored))

		shop, err := kind.Encode(stackKind.Stack{
			Kind:     stackKind.Name,
			Metadata: kind.Metadata{UUID: "stack-uuid", Name: "shop", Slug: "shop-abcde", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
			Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-1"}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
			Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running}},
		})
		require.NoError(t, err)

		w.Keep(shop)

		labels := labelled(t, "from-the-snapshot")
		labels[stackKind.LabelStack] = "stack-uuid"
		labels[stackKind.LabelService] = "web"

		witnessed(t, w, at, seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "shop-abcde_web", State: "running", Labels: labels}))

		adopted, kept := w.Kept(containerKind.Name, "from-the-snapshot")
		require.True(t, kept)

		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}, adopted.Metadata.Owners, "its stack's")
		assert.Equal(t, map[string]string{stackKind.LabelStack: "stack-uuid", stackKind.LabelService: "web"}, adopted.Metadata.Labels)
		assert.Equal(t, "shop-abcde_web", adopted.Metadata.Name)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, adopted.Metadata.Refs, "living in its vm, which does not own it")
	})

	t.Run("but of two its labels say are the same service of a stack, only the first is taken in as the stack's, and one of a service its file has not is nobody's", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
		w := blockstest.New(vmtest.WithVMs(restored))

		shop, err := kind.Encode(stackKind.Stack{
			Kind:     stackKind.Name,
			Metadata: kind.Metadata{UUID: "stack-uuid", Name: "shop", Slug: "shop-abcde", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
			Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-1"}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
			Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running}},
		})
		require.NoError(t, err)

		w.Keep(shop)

		as := func(uuid string, service string) map[string]string {
			labels := labelled(t, uuid)
			labels[stackKind.LabelStack] = "stack-uuid"
			labels[stackKind.LabelService] = service

			return labels
		}

		witnessed(t, w, at,
			seen(t, "web-1", containerKind.Docker{ID: "c1", Name: "shop-abcde_web", State: "running", Labels: as("web-1", "web")}),
			seen(t, "web-2", containerKind.Docker{ID: "c2", Name: "copy-of-web", State: "running", Labels: as("web-2", "web")}),
			seen(t, "bogus", containerKind.Docker{ID: "c3", Name: "anything", State: "running", Labels: as("bogus", "not-in-its-file")}),
		)

		first, kept := w.Kept(containerKind.Name, "web-1")
		require.True(t, kept)
		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}, first.Metadata.Owners)
		assert.Equal(t, "shop-abcde_web", first.Metadata.Name)

		for _, uuid := range []string{"web-2", "bogus"} {
			r, kept := w.Kept(containerKind.Name, uuid)
			require.True(t, kept, "%s is taken in", uuid)
			assert.Empty(t, r.Metadata.Owners, "nobody's: %s", uuid)
			assert.NotEqual(t, "shop-abcde_web", r.Metadata.Name, "and not named as the service's: %s", uuid)
		}
	})

	t.Run("but one taken in for a stack forgotten meanwhile, as its vm's restored disk was found not to hold it, is forgotten too, as what the stack owned was, and is nobody's once its node goes on saying it", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
		w := blockstest.New(vmtest.WithVMs(restored))

		shop, err := kind.Encode(stackKind.Stack{
			Kind:     stackKind.Name,
			Metadata: kind.Metadata{UUID: "stack-uuid", Name: "shop", Slug: "shop-abcde", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
			Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-1"}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
			Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running}},
		})
		require.NoError(t, err)

		w.Keep(shop)

		// the stack's forget, which read nothing of the container yet, done
		// once the stack was read as there, before the container's record
		// is kept.
		forgetting := &creating{Repository: w.Resources, before: func(ctx context.Context, r resource.Record) {
			if r.Kind == containerKind.Name {
				require.NoError(t, w.Resources.Delete(ctx, stackKind.Name, "stack-uuid"))
			}
		}}

		containers := controlPlaneContainers.New(blocks.Dependencies{Resources: forgetting, VMs: w.Records, Chooser: w.Chooser, Requester: w.Requester, Sightings: w.Sightings})

		labels := labelled(t, "from-the-snapshot")
		labels[stackKind.LabelStack] = "stack-uuid"
		labels[stackKind.LabelService] = "web"

		web := seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "shop-abcde_web", State: "running", Labels: labels})

		require.NoError(t, containers.Witnessed(ctx, vmtest.Node, web, false, at))

		_, kept := w.Kept(containerKind.Name, "from-the-snapshot")
		assert.False(t, kept, "not kept owned by what is gone, which would have it removed from its vm")

		// said for as long since, from the next beat on.
		for beat := time.Second; beat <= blockstest.SureAfter+time.Second; beat += time.Second {
			require.NoError(t, containers.Witnessed(ctx, vmtest.Node, web, false, at.Add(beat)))
		}

		extras, err := containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 1)
		assert.Equal(t, "from-the-snapshot", extras[0].Metadata.UUID, "it is nobody's")
		assert.Equal(t, blockKinds.ManagedByNobody, extras[0].Metadata.Labels[blockKinds.LabelManagedBy])
	})

	t.Run("and what telling whether its stack went meanwhile fails with, and letting it go again fails with, is said, to be heard again, what was taken in kept as it was", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
		w := blockstest.New(vmtest.WithVMs(restored))

		shop, err := kind.Encode(stackKind.Stack{
			Kind:     stackKind.Name,
			Metadata: kind.Metadata{UUID: "stack-uuid", Name: "shop", Slug: "shop-abcde", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
			Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-1"}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
			Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running}},
		})
		require.NoError(t, err)

		w.Keep(shop)

		gone := errors.New("the database is gone")

		// the database gone once the container's record is kept.
		failing := &created{Repository: w.Resources, after: func(_ context.Context, r resource.Record) {
			if r.Kind == containerKind.Name {
				w.Memory.Fail = gone
			}
		}}

		containers := controlPlaneContainers.New(blocks.Dependencies{Resources: failing, VMs: w.Records, Chooser: w.Chooser, Requester: w.Requester, Sightings: w.Sightings})

		labels := labelled(t, "from-the-snapshot")
		labels[stackKind.LabelStack] = "stack-uuid"
		labels[stackKind.LabelService] = "web"

		err = containers.Witnessed(ctx, vmtest.Node, seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "shop-abcde_web", State: "running", Labels: labels}), false, at)
		assert.ErrorIs(t, err, gone)
		assert.ErrorIs(t, err, kind.ErrHearAgain)

		w.Memory.Fail = nil

		adopted, kept := w.Kept(containerKind.Name, "from-the-snapshot")
		require.True(t, kept)
		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}, adopted.Metadata.Owners)
	})

	t.Run("the first look's heartbeat that could not take in what it found is to be heard again, and heard again takes it in", func(t *testing.T) {
		t.Parallel()

		for name, blink := range map[string]func(b *blinking){
			"the stack's containers not listed, the database gone for a moment": func(b *blinking) {
				b.arm("list " + containerKind.Name)
			},
			"the look not kept on its vm's record": func(b *blinking) {
				b.arm("update " + vmKind.Name)
			},
			"its stack not read again once what it took in is kept, which is let go of again": func(b *blinking) {
				b.after("create "+containerKind.Name, "get "+stackKind.Name)
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
				w := blockstest.New(vmtest.WithVMs(restored))

				shop, err := kind.Encode(stackKind.Stack{
					Kind:     stackKind.Name,
					Metadata: kind.Metadata{UUID: "stack-uuid", Name: "shop", Slug: "shop-abcde", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
					Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-1"}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
					Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running}},
				})
				require.NoError(t, err)

				w.Keep(shop)

				blinks := &blinking{Repository: w.Resources}
				blink(blinks)

				records := records.New(blinks)
				containers := controlPlaneContainers.New(blocks.Dependencies{Resources: blinks, VMs: records, Chooser: w.Chooser, Requester: w.Requester, Sightings: w.Sightings})

				labels := labelled(t, "from-the-snapshot")
				labels[stackKind.LabelStack] = "stack-uuid"
				labels[stackKind.LabelService] = "web"

				web := seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "shop-abcde_web", State: "running", Labels: labels})

				err = containers.Witnessed(ctx, vmtest.Node, web, false, at)
				require.ErrorIs(t, err, kind.ErrHearAgain)

				_, kept := w.Kept(containerKind.Name, "from-the-snapshot")
				assert.False(t, kept, "nothing is taken in yet")

				require.NoError(t, containers.Witnessed(ctx, vmtest.Node, web, false, at), "heard again")

				adopted, kept := w.Kept(containerKind.Name, "from-the-snapshot")
				require.True(t, kept, "it is taken in, the look's heartbeat as it was")
				assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "stack-uuid"}}, adopted.Metadata.Owners, "as the stack's")

				v, _ := w.Stored("vm-1")
				assert.True(t, v.Status.LookedAt.Equal(at), "the look kept as of its beat")
			})
		}
	})

	t.Run("but what the first look's heartbeat could not read is said, and not to be heard again, as it is however often it is heard", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
		w := blockstest.New(vmtest.WithVMs(restored))

		// a stack whose record says its file is a number.
		_, err := w.Memory.Create(ctx, resource.Record{Raw: kind.Raw{
			Kind:     stackKind.Name,
			Metadata: kind.Metadata{UUID: "stack-uuid", Name: "shop", Slug: "shop-abcde", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}},
			Spec:     json.RawMessage(`{"vm":{"uuid":"vm-1"},"compose":1}`),
			Status:   json.RawMessage(`{"state":"running","expected":"running"}`),
		}})
		require.NoError(t, err)

		labels := labelled(t, "from-the-snapshot")
		labels[stackKind.LabelStack] = "stack-uuid"
		labels[stackKind.LabelService] = "web"

		err = w.Containers.Witnessed(ctx, vmtest.Node, seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "shop-abcde_web", State: "running", Labels: labels}), false, at)
		require.Error(t, err, "said")
		assert.NotErrorIs(t, err, kind.ErrHearAgain)
	})

	t.Run("but what a heartbeat of a later beat could not do is said, and not to be heard again", func(t *testing.T) {
		t.Parallel()

		looked := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.RestoredAt = at.Add(-time.Minute)
			v.Status.LookedAt = at.Add(-time.Second)
		})
		w := blockstest.New(vmtest.WithVMs(looked))

		blinks := &blinking{Repository: w.Resources}
		blinks.arm("update " + vmKind.Name)
		blinks.arm("list " + containerKind.Name)

		containers := controlPlaneContainers.New(blocks.Dependencies{Resources: blinks, VMs: records.New(blinks), Chooser: w.Chooser, Requester: w.Requester, Sightings: w.Sightings})

		err := containers.Witnessed(ctx, vmtest.Node, seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "from-the-snapshot")}), false, at)
		assert.NotErrorIs(t, err, kind.ErrHearAgain)
	})

	t.Run("the first look into a vm since its disk was restored is kept on its record, and no look before its restore, nor one after the first", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
		w := blockstest.New(vmtest.WithVMs(restored))

		db := seen(t, "", containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		witnessed(t, w, at.Add(-2*time.Minute), db)

		v, _ := w.Stored("vm-1")
		assert.True(t, v.Status.LookedAt.IsZero(), "what a node saw before the restore says nothing of the restored disk")

		witnessed(t, w, at, db)

		v, _ = w.Stored("vm-1")
		assert.True(t, v.Status.LookedAt.Equal(at), "looked into as of the beat that first said what it holds since: %s", v.Status.LookedAt)

		witnessed(t, w, at.Add(time.Second), db)

		v, _ = w.Stored("vm-1")
		assert.True(t, v.Status.LookedAt.Equal(at), "the first look is kept, and no later one")
		assert.True(t, v.Status.RestoredAt.Equal(at.Add(-time.Minute)), "and the rest of its status as it was")
		assert.Equal(t, vmKind.Running, v.Status.State)
	})

	t.Run("one restored again since it was looked into is looked into again", func(t *testing.T) {
		t.Parallel()

		again := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.LookedAt = at.Add(-time.Hour)
			v.Status.RestoredAt = at.Add(-time.Minute)
		})
		w := blockstest.New(vmtest.WithVMs(again))

		witnessed(t, w, at, seen(t, "", containerKind.Docker{ID: "c-db", Name: "db", State: "running"}))

		v, _ := w.Stored("vm-1")
		assert.True(t, v.Status.LookedAt.Equal(at))
	})

	t.Run("one that came up again since it was first looked into is looked into again in the run it is in, which takes nothing in, nor does a heartbeat of the run before heard late", func(t *testing.T) {
		t.Parallel()

		// looked into a minute after its restore, stopped, and started again
		// a minute before at, its dockerd yet to say anything since.
		restoredAt := at.Add(-time.Hour)
		looked := restoredAt.Add(time.Minute)
		startedAgain := at.Add(-time.Minute)

		w := blockstest.New(vmtest.WithVMs(vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.RestoredAt = restoredAt
			v.Status.LookedAt = looked
			v.Status.StartedAt = startedAgain
		})))

		web := seen(t, "made-before-the-snapshot", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "made-before-the-snapshot")})

		// a heartbeat of the run before, heard late.
		witnessed(t, w, startedAgain.Add(-time.Second), web)

		v, _ := w.Stored("vm-1")
		assert.True(t, v.Status.LookedAgainAt.IsZero(), "what its dockerd said in the run before says nothing of this one")
		assert.True(t, v.Status.Looked().IsZero())

		witnessed(t, w, at, web)

		v, _ = w.Stored("vm-1")
		assert.True(t, v.Status.LookedAgainAt.Equal(at), "looked into again as of the beat that first said what it holds in this run: %s", v.Status.LookedAgainAt)
		assert.True(t, v.Status.LookedAt.Equal(looked), "its first look since its restore kept")
		assert.True(t, v.Status.Looked().Equal(at))

		witnessed(t, w, at.Add(time.Second), web)

		v, _ = w.Stored("vm-1")
		assert.True(t, v.Status.LookedAgainAt.Equal(at), "the first look into the run is kept, and no later one")

		_, kept := w.Kept(containerKind.Name, "made-before-the-snapshot")
		assert.False(t, kept, "and nothing is taken in by a look but the first one since its restore")
	})

	t.Run("and a look into a vm never restored writes nothing of it", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		before, _ := w.Kept(vmKind.Name, "vm-1")

		witnessed(t, w, at, seen(t, "", containerKind.Docker{ID: "c-db", Name: "db", State: "running"}))

		after, _ := w.Kept(vmKind.Name, "vm-1")
		assert.Equal(t, before.Version, after.Version)
	})

	t.Run("while its vm's record is yet to take its first restore, nothing is looked into nor taken in, and once it has, the next beat is the first look, which takes in what it finds", func(t *testing.T) {
		t.Parallel()

		restoring := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Restoring })
		w := blockstest.New(vmtest.WithVMs(restoring))

		web := seen(t, "from-the-new-disk", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "from-the-new-disk")})

		witnessed(t, w, at, web)

		_, kept := w.Kept(containerKind.Name, "from-the-new-disk")
		assert.False(t, kept)

		v, _ := w.Stored("vm-1")
		assert.True(t, v.Status.LookedAt.IsZero())

		w.Change("vm-1", func(v *vmKind.VM) {
			v.Status.State = vmKind.Running
			v.Status.RestoredAt = at.Add(-time.Second)
		})

		witnessed(t, w, at.Add(time.Second), web)

		_, kept = w.Kept(containerKind.Name, "from-the-new-disk")
		assert.True(t, kept, "taken in at the first look once the record says the restore")
	})

	t.Run("nor while its vm's record is yet to take a restore after one looked into", func(t *testing.T) {
		t.Parallel()

		stale := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.State = vmKind.Restoring
			v.Status.RestoredAt = at.Add(-20 * time.Minute)
			v.Status.LookedAt = at.Add(-19 * time.Minute)
		})
		w := blockstest.New(vmtest.WithVMs(stale))

		witnessed(t, w, at, seen(t, "from-the-new-disk", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "from-the-new-disk")}))

		_, kept := w.Kept(containerKind.Name, "from-the-new-disk")
		assert.False(t, kept)

		v, _ := w.Stored("vm-1")
		assert.True(t, v.Status.LookedAt.Equal(at.Add(-19*time.Minute)), "the look before is kept as it was")
	})

	t.Run("what the first look into a restored vm finds is taken in however long after the restore its dockerd came up", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Hour) })
		w := blockstest.New(vmtest.WithVMs(restored))

		witnessed(t, w, at, seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "from-the-snapshot")}))

		_, kept := w.Kept(containerKind.Name, "from-the-snapshot")
		assert.True(t, kept)
	})

	t.Run("and so does a heartbeat of that look another control plane kept, heard here first, while one of a beat after it takes nothing in, whatever was heard here before", func(t *testing.T) {
		t.Parallel()

		// another control plane heard the first look, at, and kept it on the
		// vm's record; this one heard nothing of the vm yet.
		looked := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.RestoredAt = at.Add(-time.Minute)
			v.Status.LookedAt = at
		})
		w := blockstest.New(vmtest.WithVMs(looked))

		witnessed(t, w, at.Add(15*time.Second), seen(t, "deleted-since", containerKind.Docker{ID: "c2", Name: "api", State: "running", Labels: labelled(t, "deleted-since")}))

		_, kept := w.Kept(containerKind.Name, "deleted-since")
		assert.False(t, kept, "a later beat is no first look, here or anywhere")

		witnessed(t, w, at, seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "from-the-snapshot")}))

		_, kept = w.Kept(containerKind.Name, "from-the-snapshot")
		assert.True(t, kept, "the first look's heartbeats are, whichever control plane hears them")
	})

	t.Run("what a vm nobody keeps a record of holds is not shown, nor what one holds that its record says is on another node", func(t *testing.T) {
		t.Parallel()

		elsewhere := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Metadata.Node = "node-2" })

		for name, w := range map[string]*blockstest.Workload{
			"nobody's":       blockstest.New(),
			"another node's": blockstest.New(vmtest.WithVMs(elsewhere)),
		} {
			witnessed(t, w, at, seen(t, "", containerKind.Docker{ID: "c1", Name: "db", State: "running"}))

			extras, err := w.Containers.All(ctx)
			require.NoError(t, err)
			assert.Empty(t, extras, name)
		}
	})
}

// creating keeps records as the repository it wraps does, but for doing what
// before says just before each record is created.
type creating struct {
	resource.Repository

	before func(ctx context.Context, r resource.Record)
}

func (c *creating) Create(ctx context.Context, r resource.Record) (resource.Record, error) {
	c.before(ctx, r)

	return c.Repository.Create(ctx, r)
}

// blinking keeps records as the repository it wraps does, but for failing
// the next of each call armed, once, as the database gone for a moment: a
// call is named by what it does, get, list, create, update or delete, and the
// kind it does it to.
type blinking struct {
	resource.Repository

	lock  sync.Mutex
	armed map[string]bool

	// then are what is armed once each call named is done.
	then map[string]string
}

// arm has the next call op names fail.
func (b *blinking) arm(op string) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.armed == nil {
		b.armed = make(map[string]bool)
	}

	b.armed[op] = true
}

// after has the call next names armed once a call op names is done.
func (b *blinking) after(op string, next string) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.then == nil {
		b.then = make(map[string]string)
	}

	b.then[op] = next
}

// blinks reports whether the call op names fails, which it does once when it
// is armed.
func (b *blinking) blinks(op string) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if !b.armed[op] {
		return nil
	}

	delete(b.armed, op)

	return errors.New("the database is gone for a moment")
}

// done arms what is to be armed once the call op names is done.
func (b *blinking) done(op string) {
	b.lock.Lock()
	next, then := b.then[op]
	delete(b.then, op)
	b.lock.Unlock()

	if then {
		b.arm(next)
	}
}

func (b *blinking) GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error) {
	if err := b.blinks("get " + kindName); err != nil {
		return resource.Record{}, err
	}

	return b.Repository.GetOne(ctx, kindName, uuid)
}

func (b *blinking) GetAll(ctx context.Context, kindName string, filter resource.Filter, offset uint, limit uint) ([]resource.Record, uint, error) {
	if err := b.blinks("list " + kindName); err != nil {
		return nil, 0, err
	}

	return b.Repository.GetAll(ctx, kindName, filter, offset, limit)
}

func (b *blinking) Create(ctx context.Context, r resource.Record) (resource.Record, error) {
	if err := b.blinks("create " + r.Kind); err != nil {
		return resource.Record{}, err
	}

	made, err := b.Repository.Create(ctx, r)
	if err == nil {
		b.done("create " + r.Kind)
	}

	return made, err
}

func (b *blinking) Update(ctx context.Context, r resource.Record) (resource.Record, error) {
	if err := b.blinks("update " + r.Kind); err != nil {
		return resource.Record{}, err
	}

	return b.Repository.Update(ctx, r)
}

// created keeps records as the repository it wraps does, but for doing what
// after says just after each record is created.
type created struct {
	resource.Repository

	after func(ctx context.Context, r resource.Record)
}

func (c *created) Create(ctx context.Context, r resource.Record) (resource.Record, error) {
	made, err := c.Repository.Create(ctx, r)
	if err == nil {
		c.after(ctx, r)
	}

	return made, err
}

// labelled are the labels a container the platform made as uuid has, its
// spec among them.
func labelled(t *testing.T, uuid string) map[string]string {
	t.Helper()

	spec, err := containerKind.SpecLabel(containerKind.Spec{Name: "web", Image: "nginx:1.27"})
	require.NoError(t, err)

	labels := blockKinds.Labels(containerKind.Name, uuid)
	labels[containerKind.LabelSpec] = spec

	return labels
}

func TestBlocks_Act(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	sighted := func(t *testing.T, w *blockstest.Workload, docker containerKind.Docker) kind.Raw {
		t.Helper()

		witnessed(t, w, time.Now(), seen(t, "", docker))

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 1)

		return extras[0]
	}

	t.Run("it is asked of its node at once, and is what the command left it as", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := sighted(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		w.Answer(func(query kind.Query) kind.ResourceActedOn {
			status, _ := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.Completed}, Docker: &containerKind.Docker{ID: "c-db", Name: "db", State: "exited"}})

			return kind.ResourceActedOn{OK: true, Kind: query.Kind, UUID: query.UUID, Action: query.Action, Status: status}
		}, nil)

		after, gone, refused, err := w.Containers.Act(ctx, db, containerKind.ActionStop, nil)
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.False(t, gone)

		assert.Equal(t, "exited", blocks.ObservedOf(after.Status).Docker.State)

		asked := w.Requester.Asked()
		require.Len(t, asked, 1)
		assert.Equal(t, vmtest.Node, asked[0].NodeName)
		assert.Equal(t, kind.Op(containerKind.Name, containerKind.ActionStop), asked[0].Request.Op)

		shown, err := w.Containers.One(ctx, db.Metadata.UUID)
		require.NoError(t, err)
		assert.Equal(t, after, shown, "and is shown so until its node says otherwise")
	})

	t.Run("removed, it is gone", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := sighted(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "exited"})

		w.Answer(func(query kind.Query) kind.ResourceActedOn { return kind.ResourceActedOn{OK: true} }, nil)

		before := time.Now()

		_, gone, _, err := w.Containers.Act(ctx, db, containerKind.ActionDelete, nil)
		require.NoError(t, err)
		assert.True(t, gone)

		_, err = w.Containers.One(ctx, db.Metadata.UUID)
		assert.ErrorIs(t, err, domain.ErrNotExists)

		witnessed(t, w, before, seen(t, "", containerKind.Docker{ID: "c-db", Name: "db", State: "exited"}))

		_, err = w.Containers.One(ctx, db.Metadata.UUID)
		assert.ErrorIs(t, err, domain.ErrNotExists, "and what its node saw before it was removed does not bring it back")
	})

	t.Run("one that runs is not removed but by force, and its node is not asked", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := sighted(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		_, _, _, err := w.Containers.Act(ctx, db, containerKind.ActionDelete, nil)

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, noderequest.CodeInvalid, refused.Code)
		assert.Empty(t, w.Requester.Asked())
	})

	t.Run("what its node refused is the node's refusal, in the codes every side knows", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := sighted(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "exited"})

		w.Answer(func(query kind.Query) kind.ResourceActedOn {
			status, _ := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.Failed}, Failure: &noderequest.Error{Code: noderequest.CodeInvalid, Message: "port is already allocated"}})

			return kind.ResourceActedOn{OK: false, Reason: "port is already allocated", Status: status}
		}, nil)

		_, _, _, err := w.Containers.Act(ctx, db, containerKind.ActionStart, nil)

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, &noderequest.Error{Code: noderequest.CodeInvalid, Message: "port is already allocated"}, refused)
	})

	t.Run("what its state does not allow is refused as a record's is", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := sighted(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		_, _, refused, err := w.Containers.Act(ctx, db, containerKind.ActionStart, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"action": "invalid_state_transition"}, refused)
	})

	t.Run("and the workload's own commands are nobody's to ask", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := sighted(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		_, _, _, err := w.Containers.Act(ctx, db, containerKind.ActionCreate, nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})
}

func TestBlocks_Query(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

	witnessed(t, w, time.Now(),
		seen(t, "", containerKind.Docker{ID: "c-db", Name: "db", State: "running"}),
		seen(t, "", containerKind.Docker{ID: "c-old", Name: "old", State: "exited"}),
	)

	w.Answer(nil, json.RawMessage(`{"lines":[{"line":"ready"}]}`))

	db, err := w.Containers.One(ctx, blockKinds.Derived(containerKind.Name, "vm-1", "c-db"))
	require.NoError(t, err)

	answer, refused, err := w.Containers.Query(ctx, db, containerKind.ActionLogs, []byte(`{"tail": 1}`))
	require.NoError(t, err)
	require.Empty(t, refused)
	assert.JSONEq(t, `{"lines":[{"line":"ready"}]}`, string(answer))

	asked := w.Requester.Asked()
	require.Len(t, asked, 1)

	query, err := kind.QueryOf(asked[0].Request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tail":1}`, string(query.Payload))
	assert.Equal(t, db.Metadata.UUID, query.Resource.Metadata.UUID, "asked about what it is, as it was seen")

	old, err := w.Containers.One(ctx, blockKinds.Derived(containerKind.Name, "vm-1", "c-old"))
	require.NoError(t, err)

	_, _, err = w.Containers.Query(ctx, old, containerKind.ActionStats, nil)

	var notRunning *noderequest.Error
	require.ErrorAs(t, err, &notRunning)
	assert.Equal(t, noderequest.CodeNotRunning, notRunning.Code, "what one that is not running uses is not sampled")
}

func TestBlocks_Resolve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))
	w.Keep(blockstest.AContainer("kept", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
		c.Status.Docker.ID = "0123456789abcdef"
	}))
	w.Keep(blockstest.AContainer("pending", "vm-1", containerKind.Pending, containerKind.Running, func(c *containerKind.Container) {
		c.Spec.Name = "api"
		c.Status.Docker = nil
	}))
	w.Keep(blockstest.AContainer("elsewhere", "vm-2", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
		c.Spec.Name = "cache"
		c.Status.Docker.ID = "fedcba"
		c.Status.Docker.Name = "cache"
	}))

	witnessed(t, w, time.Now(), seen(t, "", containerKind.Docker{ID: "0123ffff", Name: "db", State: "running"}))

	unmanaged := blockKinds.Derived(containerKind.Name, "vm-1", "0123ffff")
	vm1 := kind.Reference{Kind: "vm", UUID: "vm-1"}

	for name, tt := range map[string]struct {
		name string
		want string
	}{
		"a record by its uuid": {name: "kept", want: "kept"},
		"by its docker id":     {name: "0123456789abcdef", want: "kept"},
		"by its name":          {name: "web", want: "kept"},
		"by the name it was asked for with, not made yet": {name: "api", want: "pending"},
		"by the one docker id beginning so":               {name: "012345", want: "kept"},
		"what nobody keeps a record of, by its name":      {name: "db", want: unmanaged},
		"and by its docker id":                            {name: "0123ffff", want: unmanaged},
		"and by its uuid":                                 {name: unmanaged, want: unmanaged},
		"but not by a beginning several ids have":         {name: "0123"},
		"nor by one too short to name any":                {name: "01"},
		"nor what lives in another vm":                    {name: "cache"},
		"nor nothing":                                     {name: " "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			uuid, err := w.Containers.Resolve(ctx, vm1, tt.name)

			if len(tt.want) == 0 {
				assert.ErrorIs(t, err, domain.ErrNotExists)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, uuid)
		})
	}
}

func TestBlocks_Into(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stopped := vmtest.In(vmtest.Docker("vm-stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })
	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), stopped, vmtest.Docker("theirs", "somebody-else")))

	v, refused, err := w.Images.Into(ctx, "owner", "vm-1")
	require.NoError(t, err)
	require.Empty(t, refused)
	assert.Equal(t, "vm-1", v.Metadata.UUID)

	for parent, want := range map[string]domain.ValidationErrors{
		"":           {"vm": "required_field"},
		"vm-stopped": {"vm": "not_running"},
		"theirs":     {"vm.uuid": "not_found"},
	} {
		_, refused, err := w.Images.Into(ctx, "owner", parent)
		require.NoError(t, err)
		assert.Equal(t, want, refused, parent)
	}
}

func TestBlocks_Present(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stopped := vmtest.In(vmtest.Docker("vm-stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })
	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), stopped))

	create := []kind.Intent{{Action: blockKinds.ActionCreate, Reason: "it is not in its vm, which runs"}}

	for name, tt := range map[string]struct {
		state    kind.State
		expected kind.State
		vm       string
		node     string
		want     []kind.Intent
	}{
		"one not made yet is made":          {state: networkKind.Pending, expected: networkKind.Present, vm: "vm-1", node: vmtest.Node, want: create},
		"and one its vm lost":               {state: networkKind.Missing, expected: networkKind.Present, vm: "vm-1", node: vmtest.Node, want: create},
		"but not while its vm does not run": {state: networkKind.Missing, expected: networkKind.Present, vm: "vm-stopped", node: vmtest.Node},
		"nor in a vm that is gone":          {state: networkKind.Missing, expected: networkKind.Present, vm: "vm-gone", node: vmtest.Node},
		"nor one on no node, which went with its vm, its graph's to place and make": {state: networkKind.Missing, expected: networkKind.Present, vm: "vm-1"},
		"nor one on a node its vm is not on, its vm made again elsewhere":           {state: networkKind.Missing, expected: networkKind.Present, vm: "vm-1", node: "workload-orchestrator-02"},
		"one that is there is left":       {state: networkKind.Present, expected: networkKind.Present, vm: "vm-1", node: vmtest.Node},
		"one waiting on its vm waits":     {state: networkKind.Waiting, expected: networkKind.Present, vm: "vm-1", node: vmtest.Node},
		"one to be deleted is the loop's": {state: networkKind.Missing, expected: kind.Deleted, vm: "vm-1", node: vmtest.Node},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			intents, err := w.Networks.Present(ctx, tt.state, tt.expected, tt.vm, tt.node)
			require.NoError(t, err)
			assert.Equal(t, tt.want, intents)
		})
	}
}

func TestObservedOf(t *testing.T) {
	t.Parallel()

	status, err := json.Marshal(containerKind.Status{
		Status: kind.Status{State: containerKind.Stopped},
		Docker: &containerKind.Docker{ID: "c1", Name: "web", State: "exited", Labels: map[string]string{"workload.stack": "stack-uuid"}},
	})
	require.NoError(t, err)

	observed := blocks.ObservedOf(status)
	assert.Equal(t, containerKind.Stopped, observed.Status.State, "what a container is doing, in the kind's words")
	assert.Equal(t, "exited", observed.Docker.State, "and in docker's, beside it")
	assert.Equal(t, "c1", observed.Docker.ID)
	assert.Equal(t, "web", observed.Docker.Name)
	assert.Equal(t, "stack-uuid", observed.Docker.Labels["workload.stack"])

	unseen := blocks.ObservedOf(json.RawMessage(`{"state": "pending"}`))
	assert.Equal(t, containerKind.Pending, unseen.Status.State)
	require.NotNil(t, unseen.Docker, "what docker has said nothing of says nothing")
	assert.Empty(t, unseen.Docker.ID)
}

func TestRefusal(t *testing.T) {
	t.Parallel()

	status, err := json.Marshal(imageKind.Status{Failure: &noderequest.Error{Code: noderequest.CodeDockerUnavailable, Message: "its dockerd did not answer"}})
	require.NoError(t, err)

	assert.Equal(t, &noderequest.Error{Code: noderequest.CodeDockerUnavailable, Message: "its dockerd did not answer"}, blocks.Refusal(status, "anything"))
	assert.Equal(t, &noderequest.Error{Code: noderequest.CodeInternal, Message: "it broke"}, blocks.Refusal(nil, "it broke"), "and when it says nothing, its reason")
}
