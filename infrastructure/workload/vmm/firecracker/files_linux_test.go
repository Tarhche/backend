//go:build linux

package firecracker

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// testExecName is what the firecrackers in these tests are called. It is
// nobody else's, so a real one running beside the tests is never taken for
// theirs.
const testExecName = "fc-vmm-test"

// machineUser is who a machine in these tests runs as when it runs as a user
// of its own: whoever runs the tests, since nothing else can be given anything
// by them, unless that is root, who can give anything to anybody.
func machineUser() int {
	if uid := os.Getuid(); uid > 0 {
		return uid
	}

	return 1_000_000_007
}

// shortDir is a directory of the test's own somewhere short: machines' sockets
// are reached by their whole paths, which have to fit in a unix socket's
// address, and a test's own temporary directory can be most of that already.
func shortDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "fc")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}

// givable skips a test that gives files to a machine's user where that cannot
// be done: whoever is not root can only give a file to itself, and to a group
// of the same number only if it is in one.
func givable(t *testing.T) {
	t.Helper()

	uid := os.Getuid()
	if uid == 0 {
		return
	}

	groups, err := os.Getgroups()
	require.NoError(t, err)

	if os.Getgid() != uid && !slices.Contains(groups, uid) {
		t.Skipf("user %d cannot give a file to user and group %d, which only root could", uid, uid)
	}
}

// hypervisor builds a hypervisor over a data directory of its own, laid out,
// whose firecracker is only a file: nothing here starts one. Machines run as
// users of their own when users says so, of which there is one: machineUser.
func hypervisor(t *testing.T, users bool) (*Hypervisor, string) {
	t.Helper()

	dataDir := filepath.Join(shortDir(t), "vmhost")
	require.NoError(t, layout.Prepare(dataDir))

	binary := filepath.Join(layout.Bin(dataDir), testExecName)
	require.NoError(t, os.WriteFile(binary, nil, 0o755))

	config := Config{DataDir: dataDir, Binary: binary, Mode: ModeChild}
	if users {
		config.FirstUID, config.UIDs = machineUser(), 1
	}

	return &Hypervisor{
		config:   config,
		logger:   slog.New(slog.DiscardHandler),
		binary:   binary,
		execName: testExecName,
		launcher: &children{dataDir: dataDir, execName: testExecName, byUser: users},
	}, dataDir
}

// files writes what a machine boots from into the data directory, everybody's
// to read, and says where each is.
func files(t *testing.T, dataDir string) vm.MachineSpec {
	t.Helper()

	s := vm.MachineSpec{
		ID:        "0123456789abcdef",
		VCPUs:     1,
		MemoryMiB: 128,
		Kernel:    filepath.Join(layout.Boot(dataDir), "vmlinux-0123456789abcdef"),
		Initrd:    filepath.Join(layout.Boot(dataDir), "initrd-0123456789abcdef.cpio.gz"),
		Drives: []vm.Drive{
			{Path: filepath.Join(layout.Images(dataDir), "rootfs.squashfs"), ReadOnly: true},
			{Path: filepath.Join(layout.VMs(dataDir), "scratch.ext4")},
		},
	}

	for _, path := range []string{s.Kernel, s.Initrd, s.Drives[0].Path, s.Drives[1].Path} {
		require.NoError(t, os.WriteFile(path, []byte(filepath.Base(path)), 0o644))
	}

	return s
}

func TestPrepare(t *testing.T) {
	t.Parallel()

	t.Run("what a machine boots from is linked into its directory, under names of its own", func(t *testing.T) {
		t.Parallel()

		h, dataDir := hypervisor(t, false)
		s := files(t, dataDir)

		require.NoError(t, h.prepare(s))

		root := layout.MachineRoot(dataDir, s.ID)

		for name, content := range map[string]string{
			"vmlinux": "vmlinux-0123456789abcdef",
			"initrd":  "initrd-0123456789abcdef.cpio.gz",
			"drive0":  "rootfs.squashfs",
			"drive1":  "scratch.ext4",
		} {
			read, err := os.ReadFile(filepath.Join(root, name))
			require.NoError(t, err, name)
			assert.Equal(t, content, string(read), name)
		}

		var linked, installed unix.Stat_t
		require.NoError(t, unix.Stat(filepath.Join(layout.Machine(dataDir, s.ID), testExecName), &linked))
		require.NoError(t, unix.Stat(h.binary, &installed))
		assert.Equal(t, installed.Ino, linked.Ino, "the machine's firecracker is the installed one itself, beside the machine's own directory")
	})

	t.Run("a machine's directory is vmhost's, and what it runs in is its user's alone", func(t *testing.T) {
		t.Parallel()
		givable(t)

		h, dataDir := hypervisor(t, true)
		s := files(t, dataDir)
		s.UID = machineUser()

		require.NoError(t, h.prepare(s))

		root := layout.MachineRoot(dataDir, s.ID)

		for path, expected := range map[string]struct {
			mode  os.FileMode
			owner int
		}{
			layout.Machine(dataDir, s.ID): {mode: 0o711, owner: os.Getuid()},
			root:                          {mode: 0o700, owner: s.UID},
			filepath.Join(root, "run"):    {mode: 0o700, owner: s.UID},
		} {
			info, err := os.Lstat(path)
			require.NoError(t, err)

			assert.Equal(t, expected.mode, info.Mode().Perm(), path)

			owner, ok := ownerOf(path)
			require.True(t, ok)
			assert.Equal(t, expected.owner, owner, path)
		}
	})

	t.Run("a machine that could not be linked fails, whatever it was asked to link", func(t *testing.T) {
		t.Parallel()

		h, dataDir := hypervisor(t, false)
		s := files(t, dataDir)
		s.Drives[1].Path = filepath.Join(layout.VMs(dataDir), "missing.ext4")

		assert.ErrorIs(t, h.prepare(s), vm.ErrInvalid)
	})
}

func TestLink(t *testing.T) {
	t.Parallel()

	t.Run("nothing outside the data directory is linked, however it is asked for", func(t *testing.T) {
		t.Parallel()

		h, dataDir := hypervisor(t, false)

		outside := filepath.Join(t.TempDir(), "secret")
		require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o644))

		for _, source := range []string{
			outside,
			filepath.Join(dataDir, "..", filepath.Base(filepath.Dir(outside)), "secret"),
			dataDir,
			"relative/path",
		} {
			assert.ErrorIs(t, h.link(source, filepath.Join(t.TempDir(), "vmlinux"), 0, shared), vm.ErrInvalid, source)
		}
	})

	t.Run("a symlink is not a file, wherever it points", func(t *testing.T) {
		t.Parallel()

		h, dataDir := hypervisor(t, false)

		real := filepath.Join(layout.Boot(dataDir), "real")
		require.NoError(t, os.WriteFile(real, nil, 0o644))
		require.NoError(t, os.Symlink(real, filepath.Join(layout.Boot(dataDir), "pointer")))

		assert.ErrorIs(t, h.link(filepath.Join(layout.Boot(dataDir), "pointer"), filepath.Join(t.TempDir(), "vmlinux"), 0, shared), vm.ErrInvalid)
	})

	t.Run("a directory on the way that is a symlink leads nowhere, even back inside", func(t *testing.T) {
		t.Parallel()

		h, dataDir := hypervisor(t, false)

		require.NoError(t, os.WriteFile(filepath.Join(layout.Boot(dataDir), "vmlinux"), nil, 0o644))
		require.NoError(t, os.Symlink(layout.Boot(dataDir), filepath.Join(dataDir, "elsewhere")))

		assert.ErrorIs(t, h.link(filepath.Join(dataDir, "elsewhere", "vmlinux"), filepath.Join(t.TempDir(), "vmlinux"), 0, shared), vm.ErrInvalid)
	})

	t.Run("a directory is not a file", func(t *testing.T) {
		t.Parallel()

		h, dataDir := hypervisor(t, false)

		assert.ErrorIs(t, h.link(layout.Boot(dataDir), filepath.Join(t.TempDir(), "vmlinux"), 0, shared), vm.ErrInvalid)
	})

	t.Run("a disk a machine writes is made its own, and nobody else's", func(t *testing.T) {
		t.Parallel()
		givable(t)

		h, dataDir := hypervisor(t, true)

		scratch := filepath.Join(layout.VMs(dataDir), "scratch.ext4")
		require.NoError(t, os.WriteFile(scratch, nil, 0o644))

		require.NoError(t, h.link(scratch, filepath.Join(t.TempDir(), "drive1"), machineUser(), own))

		info, err := os.Stat(scratch)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

		owner, _ := ownerOf(scratch)
		assert.Equal(t, machineUser(), owner)
	})

	t.Run("a file machines share has to be everybody's to read, and is left as it is", func(t *testing.T) {
		t.Parallel()

		h, dataDir := hypervisor(t, true)

		private := filepath.Join(layout.Boot(dataDir), "vmlinux")
		require.NoError(t, os.WriteFile(private, nil, 0o640))

		assert.ErrorContains(t, h.link(private, filepath.Join(t.TempDir(), "vmlinux"), machineUser(), shared), "everybody's to read")

		image := filepath.Join(layout.Images(dataDir), "rootfs.squashfs")
		require.NoError(t, os.WriteFile(image, nil, 0o644))

		require.NoError(t, h.link(image, filepath.Join(t.TempDir(), "drive0"), machineUser(), shared))

		info, err := os.Stat(image)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

		owner, _ := ownerOf(image)
		assert.Equal(t, os.Getuid(), owner, "a shared file is given to no machine")
	})

	t.Run("a file vmhost alone reads is linked for a machine that runs as vmhost", func(t *testing.T) {
		t.Parallel()

		h, dataDir := hypervisor(t, false)

		private := filepath.Join(layout.Boot(dataDir), "vmlinux")
		require.NoError(t, os.WriteFile(private, nil, 0o600))

		assert.NoError(t, h.link(private, filepath.Join(t.TempDir(), "vmlinux"), 0, shared))
	})
}

func TestInstall(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(t.TempDir(), "firecracker")
	require.NoError(t, os.WriteFile(source, []byte("firecracker v1"), 0o700))

	installed, err := install(source, dir)
	require.NoError(t, err)

	assert.Regexp(t, `/firecracker-[0-9a-f]{16}$`, installed, "named by what it holds")

	info, err := os.Stat(installed)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "every machine's user runs it")

	again, err := install(source, dir)
	require.NoError(t, err)
	assert.Equal(t, installed, again, "the same binary is installed once")

	itself, err := install(installed, dir)
	require.NoError(t, err)
	assert.Equal(t, installed, itself, "a binary installed already is taken as it is")

	require.NoError(t, os.WriteFile(source, []byte("firecracker v2"), 0o700))

	upgraded, err := install(source, dir)
	require.NoError(t, err)
	assert.NotEqual(t, installed, upgraded, "an upgrade is installed beside the binary machines may still run")

	content, err := os.ReadFile(installed)
	require.NoError(t, err)
	assert.Equal(t, "firecracker v1", string(content))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "nothing half copied is left behind")

	_, err = install(filepath.Join(t.TempDir(), "missing"), dir)
	assert.Error(t, err)

	_, err = install(t.TempDir(), dir)
	assert.ErrorContains(t, err, "not a file")
}

func TestDeviceGroups(t *testing.T) {
	t.Parallel()

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
	assert.ErrorIs(t, err, vm.ErrUnavailable)
	assert.ErrorContains(t, err, "owner alone")

	_, err = deviceGroups(filepath.Join(dir, "missing"))
	assert.ErrorIs(t, err, vm.ErrUnavailable)
}

// copyFile copies a file, with its mode.
func copyFile(t *testing.T, source string, target string) {
	t.Helper()

	in, err := os.Open(source)
	require.NoError(t, err)
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	require.NoError(t, err)

	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, out.Close())
}
