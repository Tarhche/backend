package state

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()

	directory := filepath.Join(t.TempDir(), "runs")

	store, err := NewStore(directory, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	return store, directory
}

func record(id string, created time.Time) runs.Record {
	return runs.Record{
		ID: id,
		Spec: api.RunSpec{
			Node:          "orchestrator-1",
			Name:          "web-" + id,
			Image:         "nginx:alpine",
			Command:       []string{"nginx", "-g", "daemon off;"},
			Environment:   []string{"A=1"},
			CPU:           0.25,
			Memory:        128 << 20,
			Disk:          1 << 40,
			Network:       api.NetworkPublic,
			Ports:         []uint16{80},
			RestartPolicy: "on-failure:3",
			Task:          api.Task{UUID: "task", Slug: "web", Kind: "service", Attempt: 2},
		},
		State:            api.StateExited,
		ExitCode:         137,
		Error:            runs.ReasonServiceRestarted,
		RestartCount:     3,
		CreatedAt:        created,
		StartedAt:        created.Add(time.Second),
		FinishedAt:       created.Add(time.Minute),
		HostPorts:        []api.Endpoint{{Port: 80, HostPort: 20001}},
		Sandbox:          true,
		StoppedByRequest: true,
		Resume:           true,
	}
}

func TestStore(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 4, 12, 0, 0, 123456789, time.UTC)

	t.Run("what is saved is loaded, exactly", func(t *testing.T) {
		t.Parallel()

		store, _ := newStore(t)

		saved := record("0001", created)
		require.NoError(t, store.Save(saved))

		loaded, err := store.Load()
		require.NoError(t, err)

		assert.Equal(t, []runs.Record{saved}, loaded)
	})

	t.Run("a save replaces the record before it, whole", func(t *testing.T) {
		t.Parallel()

		store, directory := newStore(t)

		first := record("0001", created)
		require.NoError(t, store.Save(first))

		second := first
		second.State = api.StateRunning
		second.Error = ""
		require.NoError(t, store.Save(second))

		loaded, err := store.Load()
		require.NoError(t, err)
		assert.Equal(t, []runs.Record{second}, loaded)

		entries, err := os.ReadDir(directory)
		require.NoError(t, err)
		require.Len(t, entries, 1, "nothing is left beside the record")

		info, err := entries[0].Info()
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("records load oldest first", func(t *testing.T) {
		t.Parallel()

		store, _ := newStore(t)

		require.NoError(t, store.Save(record("0003", created.Add(2*time.Second))))
		require.NoError(t, store.Save(record("0001", created)))
		require.NoError(t, store.Save(record("0002", created.Add(time.Second))))

		loaded, err := store.Load()
		require.NoError(t, err)

		var ids []string
		for _, r := range loaded {
			ids = append(ids, r.ID)
		}

		assert.Equal(t, []string{"0001", "0002", "0003"}, ids)
	})

	t.Run("a deleted record is gone, and deleting it again is no error", func(t *testing.T) {
		t.Parallel()

		store, _ := newStore(t)

		require.NoError(t, store.Save(record("0001", created)))
		require.NoError(t, store.Delete("0001"))
		require.NoError(t, store.Delete("0001"))

		loaded, err := store.Load()
		require.NoError(t, err)
		assert.Empty(t, loaded)
	})

	t.Run("a record that cannot be read is set aside rather than stopping the rest", func(t *testing.T) {
		t.Parallel()

		store, directory := newStore(t)

		require.NoError(t, store.Save(record("0001", created)))
		require.NoError(t, os.WriteFile(filepath.Join(directory, "0002.json"), []byte("{not json"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(directory, "0003.json"), []byte(`{"id":"0004"}`), 0o600))

		loaded, err := store.Load()
		require.NoError(t, err)

		require.Len(t, loaded, 1)
		assert.Equal(t, "0001", loaded[0].ID)

		assert.FileExists(t, filepath.Join(directory, "0002.json.corrupt"))
		assert.FileExists(t, filepath.Join(directory, "0003.json.corrupt"), "a record of another run under this one's name is not this run's")

		loaded, err = store.Load()
		require.NoError(t, err)
		assert.Len(t, loaded, 1, "what was set aside is not read again")
	})

	t.Run("what a write that died halfway left behind is cleaned up", func(t *testing.T) {
		t.Parallel()

		store, directory := newStore(t)

		leftover := filepath.Join(directory, ".0001.12345.tmp")
		require.NoError(t, os.WriteFile(leftover, []byte("{half"), 0o600))

		loaded, err := store.Load()
		require.NoError(t, err)

		assert.Empty(t, loaded)
		assert.NoFileExists(t, leftover)
	})

	t.Run("an ID that could climb out of the directory is refused", func(t *testing.T) {
		t.Parallel()

		store, _ := newStore(t)

		assert.Error(t, store.Save(record("../escape", created)))
		assert.Error(t, store.Save(record("", created)))
		assert.Error(t, store.Delete("../escape"))
	})

	t.Run("files that are not records are left alone", func(t *testing.T) {
		t.Parallel()

		store, directory := newStore(t)

		require.NoError(t, os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("hello"), 0o600))
		require.NoError(t, os.Mkdir(filepath.Join(directory, "sub.json"), 0o700))

		loaded, err := store.Load()
		require.NoError(t, err)
		assert.Empty(t, loaded)

		assert.FileExists(t, filepath.Join(directory, "notes.txt"))
	})

	t.Run("a directory that cannot be made is an error", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(file, nil, 0o600))

		_, err := NewStore(filepath.Join(file, "runs"), slog.New(slog.DiscardHandler))

		assert.Error(t, err)
	})
}
