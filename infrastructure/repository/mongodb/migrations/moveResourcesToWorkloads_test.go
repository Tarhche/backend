package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClashIn(t *testing.T) {
	t.Parallel()

	// what each kind kept in its own collection, under uuids and slugs of
	// its own, some reached under none.
	vm := keptResource{Collection: "vms", Kind: "vm", UUID: "vm-uuid", Slug: "box-abcde"}
	docker := keptResource{Collection: "vms", Kind: "vm", UUID: "docker-uuid", Slug: "docker-fghij"}
	snapshot := keptResource{Collection: "snapshots", Kind: "snapshot", UUID: "snapshot-uuid"}
	stack := keptResource{Collection: "stacks", Kind: "stack", UUID: "stack-uuid", Slug: "shop-klmno"}
	task := keptResource{Collection: "tasks", Kind: "task", UUID: "task-uuid", Slug: "request-pqrst"}
	container := keptResource{Collection: "containers", Kind: "container", UUID: "container-uuid"}
	image := keptResource{Collection: "images", Kind: "image", UUID: "image-uuid"}
	network := keptResource{Collection: "networks", Kind: "network", UUID: "network-uuid"}
	volume := keptResource{Collection: "volumes", Kind: "volume", UUID: "volume-uuid"}

	every := []keptResource{vm, docker, snapshot, stack, task, container, image, network, volume}

	// what is in workloads already, moved there by a run cut short.
	moved := func(r keptResource) keptResource {
		r.Collection = "workloads"

		return r
	}

	in := func(collection string, r keptResource) keptResource {
		r.Collection = collection

		return r
	}

	with := func(r keptResource, uuid string, slug string) keptResource {
		r.UUID, r.Slug = uuid, slug

		return r
	}

	t.Run("what every kind kept moves, none of it holding another's uuid or slug", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, clashIn(nil, every))
	})

	t.Run("what is in workloads already is the same resource, and clashes with nothing", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, clashIn([]keptResource{moved(vm), moved(stack), moved(snapshot)}, every))
	})

	t.Run("nor does what was moved before under another slug, which is the slug that stays", func(t *testing.T) {
		t.Parallel()

		renamed := moved(with(stack, stack.UUID, "renamed-uvwxy"))
		another := with(task, "another-task-uuid", stack.Slug)

		assert.NoError(t, clashIn([]keptResource{renamed}, append(every, another)))
	})

	t.Run("resources reached under no name share none, whatever their kinds", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, clashIn([]keptResource{moved(with(container, "moved-uuid", ""))}, []keptResource{snapshot, container, image, network, volume}))
	})

	for name, tt := range map[string]struct {
		there  []keptResource
		moving []keptResource
		says   []string
	}{
		"two kinds' resources under one slug fail it, naming them": {
			moving: append(every, with(stack, "web-stack-uuid", vm.Slug)),
			says:   []string{`the vm "vm-uuid" in vms`, `the stack "web-stack-uuid" in stacks`, `the slug "box-abcde"`, "nothing was moved"},
		},
		"and so does one under the slug of one in workloads already": {
			there:  []keptResource{moved(task)},
			moving: []keptResource{with(vm, vm.UUID, task.Slug)},
			says:   []string{`the task "task-uuid" in workloads`, `the vm "vm-uuid" in vms`, `the slug "request-pqrst"`},
		},
		"two kinds' resources under one uuid fail it, naming them": {
			moving: append(every, with(volume, vm.UUID, "")),
			says:   []string{`the vm "vm-uuid" in vms`, `the volume "vm-uuid" in volumes`, "share a uuid"},
		},
		"and so does one under the uuid of another kind's in workloads already": {
			there:  []keptResource{moved(with(image, stack.UUID, ""))},
			moving: every,
			says:   []string{`the image "stack-uuid" in workloads`, `the stack "stack-uuid" in stacks`, "share a uuid"},
		},
		"and so does one kept among another kind's": {
			moving: append(every, in("vms", with(stack, "lost-uuid", "lost-zabcd"))),
			says:   []string{`"lost-uuid" in vms is a "stack" rather than a vm`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := clashIn(tt.there, tt.moving)
			require.Error(t, err)

			for _, said := range tt.says {
				assert.Contains(t, err.Error(), said)
			}
		})
	}
}

func TestKindCollections(t *testing.T) {
	t.Parallel()

	collections := make(map[string]bool)
	kinds := make(map[string]bool)

	for _, kept := range kindCollections {
		collections[kept.collection] = true
		kinds[kept.kind] = true

		of, moved := kindKeptIn(kept.collection)
		assert.True(t, moved)
		assert.Equal(t, kept.kind, of)
	}

	assert.Len(t, collections, 8, "every kind's collection is moved, once")
	assert.Len(t, kinds, 8)

	_, moved := kindKeptIn("workloads")
	assert.False(t, moved, "workloads is where they are moved to")
}
