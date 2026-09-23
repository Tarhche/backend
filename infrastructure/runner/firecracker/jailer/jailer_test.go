//go:build linux

package jailer

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// vmm builds a VMM over a state directory of its own, with a firecracker that
// is only a file: nothing here starts one.
func vmm(t *testing.T) (*VMM, string) {
	t.Helper()

	state := t.TempDir()

	binary := filepath.Join(state, "firecracker")
	require.NoError(t, os.WriteFile(binary, nil, 0o755))

	v, err := New(Config{StateDir: state, FirecrackerBinary: binary, UID: os.Getuid(), GID: os.Getgid()}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	return v, state
}

func TestLinkFiles(t *testing.T) {
	t.Run("what a machine boots from is linked into its directory, under names of its own", func(t *testing.T) {
		v, state := vmm(t)

		for _, name := range []string{"vmlinux", "initrd.cpio.gz", "rootfs.ext4", "scratch.ext4"} {
			require.NoError(t, os.WriteFile(filepath.Join(state, name), []byte(name), 0o644))
		}

		root := t.TempDir()

		files, err := v.linkFiles(root, machine.Files{
			Kernel: filepath.Join(state, "vmlinux"),
			Initrd: filepath.Join(state, "initrd.cpio.gz"),
			Drives: []string{filepath.Join(state, "rootfs.ext4"), filepath.Join(state, "scratch.ext4")},
		})

		require.NoError(t, err)
		assert.Equal(t, machine.Files{Kernel: "vmlinux", Initrd: "initrd", Drives: []string{"drive0", "drive1"}}, files)

		content, err := os.ReadFile(filepath.Join(root, "drive1"))
		require.NoError(t, err)
		assert.Equal(t, "scratch.ext4", string(content))
	})

	t.Run("nothing outside the state directory is linked, however it is asked for", func(t *testing.T) {
		v, state := vmm(t)

		outside := filepath.Join(t.TempDir(), "secret")
		require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o644))

		for _, source := range []string{
			outside,
			filepath.Join(state, "..", filepath.Base(filepath.Dir(outside)), "secret"),
			"relative/path",
		} {
			assert.Error(t, v.link(source, t.TempDir(), "vmlinux"), source)
		}
	})

	t.Run("a symlink is not a file, wherever it points", func(t *testing.T) {
		v, state := vmm(t)

		require.NoError(t, os.WriteFile(filepath.Join(state, "real"), nil, 0o644))
		require.NoError(t, os.Symlink(filepath.Join(state, "real"), filepath.Join(state, "pointer")))

		assert.ErrorContains(t, v.link(filepath.Join(state, "pointer"), t.TempDir(), "vmlinux"), "not a file")
	})
}

func TestList(t *testing.T) {
	t.Run("a machine's directory without a process is a machine that is not running", func(t *testing.T) {
		v, _ := vmm(t)

		dir := v.machineDir("0123456789abcdef")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "root"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ownerFile), []byte("runner-orchestrator-01"), 0o644))

		// directories that are not machines are no business of the list.
		require.NoError(t, os.MkdirAll(filepath.Join(v.baseDir(), "not-a-machine"), 0o755))

		machines, err := v.List(context.Background())

		require.NoError(t, err)
		require.Len(t, machines, 1)
		assert.Equal(t, "0123456789abcdef", machines[0].ID)
		assert.Equal(t, "runner-orchestrator-01", machines[0].Owner)
		assert.False(t, machines[0].Running)
		assert.Equal(t, filepath.Join(dir, "root", socketPath), machines[0].Socket)
	})

	t.Run("a machine killed that is not running is only its directory taken away", func(t *testing.T) {
		v, _ := vmm(t)

		dir := v.machineDir("0123456789abcdef")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "root"), 0o755))

		require.NoError(t, v.Kill(context.Background(), "0123456789abcdef"))

		_, err := os.Stat(dir)
		assert.ErrorIs(t, err, os.ErrNotExist)

		require.NoError(t, v.Kill(context.Background(), "0123456789abcdef"), "a machine that is not there is the outcome asked for")
	})
}

func TestIsZombie(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("42 (fire cracker) Z 1 2 3"), 0o644))
	assert.True(t, isZombie(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("42 (firecracker) S 1 2 3"), 0o644))
	assert.False(t, isZombie(dir))
}
