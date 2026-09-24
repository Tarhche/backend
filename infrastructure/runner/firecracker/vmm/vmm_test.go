//go:build linux

package vmm

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// execName is what the firecracker in these tests is called. It is nobody
// else's, so a real one running beside the tests is never taken for theirs.
const execName = "fc-vmm-test"

// vmm builds a VMM over a state directory of its own that runs every machine
// as whoever runs the test, with a firecracker that is only a file: nothing
// here starts one.
func vmm(t *testing.T) (*VMM, string) {
	t.Helper()

	state := t.TempDir()

	binary := filepath.Join(state, execName)
	require.NoError(t, os.WriteFile(binary, nil, 0o755))

	v, err := New(Config{StateDir: state, FirecrackerBinary: binary}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	return v, state
}

// ownUsersVMM builds a VMM that gives every machine a user of its own, of which
// there is one: whoever runs the test, since nothing here can be anybody else.
func ownUsersVMM(t *testing.T) (*VMM, string) {
	t.Helper()

	state := t.TempDir()

	v := &VMM{
		config:   Config{StateDir: state, FirecrackerBinary: filepath.Join(state, execName), FirstUID: os.Getuid(), UIDs: 1, ClientGID: os.Getgid()},
		logger:   slog.New(slog.DiscardHandler),
		execName: execName,
	}

	require.NoError(t, os.MkdirAll(v.baseDir(), 0o711))

	return v, state
}

// firecracker starts a process the VMM takes for a machine's firecracker, which
// only sleeps, and ends it with the test.
func firecracker(t *testing.T, dir string) *exec.Cmd {
	t.Helper()

	binary := filepath.Join(t.TempDir(), execName)

	sleep, err := os.ReadFile("/bin/sleep")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(binary, sleep, 0o755))

	return started(t, binary, dir)
}

// started starts a command in dir, and ends it with the test.
func started(t *testing.T, binary string, dir string) *exec.Cmd {
	t.Helper()

	command := exec.Command(binary, "60")
	command.Dir = dir
	require.NoError(t, command.Start())

	exited := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(exited)
	}()

	t.Cleanup(func() {
		_ = command.Process.Kill()
		<-exited
	})

	return command
}

func TestLinkFiles(t *testing.T) {
	t.Run("what a machine boots from is linked into its directory, under names of its own", func(t *testing.T) {
		v, state := vmm(t)

		for _, name := range []string{"vmlinux", "initrd.cpio.gz", "rootfs.squashfs", "scratch.ext4"} {
			require.NoError(t, os.WriteFile(filepath.Join(state, name), []byte(name), 0o644))
		}

		root := t.TempDir()

		files, err := v.linkFiles(root, os.Getuid(), machine.Files{
			Kernel: filepath.Join(state, "vmlinux"),
			Initrd: filepath.Join(state, "initrd.cpio.gz"),
			Drives: []machine.Drive{
				{Path: filepath.Join(state, "rootfs.squashfs"), ReadOnly: true},
				{Path: filepath.Join(state, "scratch.ext4")},
			},
		})

		require.NoError(t, err)
		assert.Equal(t, machine.Files{
			Kernel: "vmlinux",
			Initrd: "initrd",
			Drives: []machine.Drive{{Path: "drive0", ReadOnly: true}, {Path: "drive1"}},
		}, files)

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
			assert.Error(t, v.link(source, t.TempDir(), "vmlinux", os.Getuid(), true), source)
		}
	})

	t.Run("a symlink is not a file, wherever it points", func(t *testing.T) {
		v, state := vmm(t)

		require.NoError(t, os.WriteFile(filepath.Join(state, "real"), nil, 0o644))
		require.NoError(t, os.Symlink(filepath.Join(state, "real"), filepath.Join(state, "pointer")))

		assert.ErrorContains(t, v.link(filepath.Join(state, "pointer"), t.TempDir(), "vmlinux", os.Getuid(), true), "not a file")
	})

	t.Run("a disk a machine writes is made its own, and nobody else's", func(t *testing.T) {
		v, state := ownUsersVMM(t)

		scratch := filepath.Join(state, "scratch.ext4")
		require.NoError(t, os.WriteFile(scratch, nil, 0o644))

		require.NoError(t, v.link(scratch, t.TempDir(), "drive1", os.Getuid(), false))

		info, err := os.Stat(scratch)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("a file machines share has to be everybody's to read, and is left as it is", func(t *testing.T) {
		v, state := ownUsersVMM(t)

		private := filepath.Join(state, "vmlinux")
		require.NoError(t, os.WriteFile(private, nil, 0o640))

		assert.ErrorContains(t, v.link(private, t.TempDir(), "vmlinux", os.Getuid(), true), "everybody's to read")

		shared := filepath.Join(state, "rootfs.squashfs")
		require.NoError(t, os.WriteFile(shared, nil, 0o644))

		require.NoError(t, v.link(shared, t.TempDir(), "drive0", os.Getuid(), true))

		info, err := os.Stat(shared)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	})
}

func TestMakeDirectory(t *testing.T) {
	v, _ := ownUsersVMM(t)

	require.NoError(t, v.makeDirectory("0123456789abcdef", os.Getuid()))

	for path, mode := range map[string]os.FileMode{
		v.machineDir("0123456789abcdef"):                    0o711,
		v.rootDir("0123456789abcdef"):                       0o750,
		filepath.Join(v.rootDir("0123456789abcdef"), "run"): 0o770 | os.ModeSetgid,
	} {
		info, err := os.Stat(path)
		require.NoError(t, err)

		assert.Equal(t, mode, info.Mode()&(os.ModePerm|os.ModeSetgid), path)
	}

	var stat unix.Stat_t
	require.NoError(t, unix.Stat(filepath.Join(v.rootDir("0123456789abcdef"), "run"), &stat))
	assert.Equal(t, uint32(os.Getgid()), stat.Gid, "what the machine makes there is the orchestrators' to open")
}

func TestList(t *testing.T) {
	t.Run("a machine's directory without a process is a machine that is not running", func(t *testing.T) {
		v, _ := vmm(t)

		dir := v.machineDir("0123456789abcdef")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "root"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ownerFile), []byte("runner-orchestrator-01"), 0o600))

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

func TestProcesses(t *testing.T) {
	t.Run("a machine that runs as a user of its own is found by that user, wherever it runs", func(t *testing.T) {
		v, _ := ownUsersVMM(t)

		require.NoError(t, os.MkdirAll(v.rootDir("0123456789abcdef"), 0o750))

		running := firecracker(t, "/")

		assert.Equal(t, map[string]int{"0123456789abcdef": running.Process.Pid}, v.processes())
	})

	t.Run("what else runs as a machine's user is not taken for its firecracker", func(t *testing.T) {
		v, _ := ownUsersVMM(t)

		require.NoError(t, os.MkdirAll(v.rootDir("0123456789abcdef"), 0o750))

		started(t, "/bin/sleep", "/")

		assert.Empty(t, v.processes())
	})

	t.Run("a machine that runs as the launcher itself is found by the directory it was started in", func(t *testing.T) {
		v, _ := vmm(t)

		root := v.rootDir("0123456789abcdef")
		require.NoError(t, os.MkdirAll(root, 0o755))

		running := firecracker(t, root)
		firecracker(t, t.TempDir())

		assert.Equal(t, map[string]int{"0123456789abcdef": running.Process.Pid}, v.processes())
	})

	t.Run("a machine is killed by its process, and its directory goes after it", func(t *testing.T) {
		v, _ := ownUsersVMM(t)

		require.NoError(t, os.MkdirAll(v.rootDir("0123456789abcdef"), 0o750))
		firecracker(t, "/")

		require.NoError(t, v.Kill(context.Background(), "0123456789abcdef"))

		assert.Empty(t, v.processes())

		_, err := os.Stat(v.machineDir("0123456789abcdef"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestLowestFree(t *testing.T) {
	user, ok := lowestFree(1000000000, 3, map[int]bool{1000000000: true, 1000000002: true})
	assert.True(t, ok)
	assert.Equal(t, 1000000001, user)

	_, ok = lowestFree(1000000000, 2, map[int]bool{1000000000: true, 1000000001: true})
	assert.False(t, ok, "every user is taken")
}

func TestDeviceGroups(t *testing.T) {
	dir := t.TempDir()

	device := func(name string, mode os.FileMode) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, nil, mode))
		require.NoError(t, os.Chmod(path, mode))

		return path
	}

	groups, err := deviceGroups(device("everybody", 0o666), device("group", 0o660))
	require.NoError(t, err)
	assert.Equal(t, []uint32{uint32(os.Getgid())}, groups, "a device open to its group takes the group; one open to everybody takes none")

	_, err = deviceGroups(device("owner", 0o600))
	assert.ErrorContains(t, err, "owner alone")

	_, err = deviceGroups(filepath.Join(dir, "missing"))
	assert.Error(t, err)
}

func TestRealUser(t *testing.T) {
	user, ok := realUser(os.Getpid())

	assert.True(t, ok)
	assert.Equal(t, os.Getuid(), user)
}

func TestIsZombie(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("42 (fire cracker) Z 1 2 3"), 0o644))
	assert.True(t, isZombie(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("42 (firecracker) S 1 2 3"), 0o644))
	assert.False(t, isZombie(dir))
}
