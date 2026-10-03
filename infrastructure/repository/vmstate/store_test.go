package vmstate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

func record(id string, createdAt time.Time) vm.VM {
	return vm.VM{
		ID:        id,
		State:     vm.StateCreated,
		CreatedAt: createdAt,
		Spec: vm.Spec{
			Name:     "nginx-" + id[:4],
			Image:    "nginx:alpine",
			Labels:   map[string]string{"node.name": "orchestrator-01"},
			Networks: []vm.Attachment{{Network: "workload-isolated", Aliases: []string{"web"}}},
		},
	}
}

func TestStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("records are kept across a store being opened again, oldest first", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()

		store, err := Open(dataDir)
		require.NoError(t, err)

		now := time.Now().UTC().Truncate(time.Second)

		require.NoError(t, store.Put(ctx, record("bbbbbbbbbbbbbbbb", now)))
		require.NoError(t, store.Put(ctx, record("aaaaaaaaaaaaaaaa", now.Add(time.Second))))

		updated, err := store.Update(ctx, "bbbbbbbbbbbbbbbb", func(v *vm.VM) { v.State = vm.StateRunning })
		require.NoError(t, err)
		assert.Equal(t, vm.StateRunning, updated.State)

		reopened, err := Open(dataDir)
		require.NoError(t, err)

		all, err := reopened.All(ctx)
		require.NoError(t, err)
		require.Len(t, all, 2)
		assert.Equal(t, "bbbbbbbbbbbbbbbb", all[0].ID)
		assert.Equal(t, vm.StateRunning, all[0].State)
		assert.Equal(t, "aaaaaaaaaaaaaaaa", all[1].ID)
		assert.Equal(t, record("aaaaaaaaaaaaaaaa", now.Add(time.Second)), all[1])

		info, err := os.Stat(layout.State(dataDir, "aaaaaaaaaaaaaaaa"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a record is vmhost's alone")
	})

	t.Run("what is handed out shares nothing with what is kept", func(t *testing.T) {
		t.Parallel()

		store, err := Open(t.TempDir())
		require.NoError(t, err)

		require.NoError(t, store.Put(ctx, record("cccccccccccccccc", time.Now())))

		got, err := store.Get(ctx, "cccccccccccccccc")
		require.NoError(t, err)

		got.Spec.Labels["node.name"] = "somebody-else"
		got.Spec.Networks[0].Aliases[0] = "changed"

		again, err := store.Get(ctx, "cccccccccccccccc")
		require.NoError(t, err)
		assert.Equal(t, "orchestrator-01", again.Spec.Labels["node.name"])
		assert.Equal(t, "web", again.Spec.Networks[0].Aliases[0])
	})

	t.Run("a vm that is not there is not found, and removing it is what was asked", func(t *testing.T) {
		t.Parallel()

		store, err := Open(t.TempDir())
		require.NoError(t, err)

		_, err = store.Get(ctx, "dddddddddddddddd")
		assert.ErrorIs(t, err, vm.ErrNotFound)

		_, err = store.Update(ctx, "dddddddddddddddd", func(*vm.VM) {})
		assert.ErrorIs(t, err, vm.ErrNotFound)

		assert.NoError(t, store.Remove(ctx, "dddddddddddddddd"))
	})

	t.Run("removing a vm takes everything kept beside its record with it", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()

		store, err := Open(dataDir)
		require.NoError(t, err)

		require.NoError(t, store.Put(ctx, record("eeeeeeeeeeeeeeee", time.Now())))
		require.NoError(t, os.WriteFile(layout.Scratch(dataDir, "eeeeeeeeeeeeeeee"), []byte("disk"), 0o600))

		require.NoError(t, store.Remove(ctx, "eeeeeeeeeeeeeeee"))

		_, err = os.Stat(layout.VM(dataDir, "eeeeeeeeeeeeeeee"))
		assert.ErrorIs(t, err, os.ErrNotExist)

		_, err = store.Get(ctx, "eeeeeeeeeeeeeeee")
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("nothing is kept or removed under what cannot name a vm", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()

		store, err := Open(dataDir)
		require.NoError(t, err)

		assert.ErrorIs(t, store.Put(ctx, vm.VM{ID: "../escape"}), vm.ErrInvalid)

		precious := filepath.Join(dataDir, "precious")
		require.NoError(t, os.WriteFile(precious, nil, 0o600))
		require.NoError(t, store.Remove(ctx, "../precious"))

		_, err = os.Stat(precious)
		assert.NoError(t, err)
	})

	t.Run("a directory whose record was never written is let go when the store is opened", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()
		require.NoError(t, os.MkdirAll(layout.VM(dataDir, "ffffffffffffffff"), 0o700))

		store, err := Open(dataDir)
		require.NoError(t, err)

		all, err := store.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, all)

		_, err = os.Stat(layout.VM(dataDir, "ffffffffffffffff"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("a record that cannot be read stops the store from opening", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()
		require.NoError(t, os.MkdirAll(layout.VM(dataDir, "0000000000000000"), 0o700))
		require.NoError(t, os.WriteFile(layout.State(dataDir, "0000000000000000"), []byte("{"), 0o600))

		_, err := Open(dataDir)
		assert.Error(t, err)
	})
}
