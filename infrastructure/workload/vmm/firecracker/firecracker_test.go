package firecracker

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// config is what vmhost runs machines with in production, in the units mode.
func config() Config {
	return Config{
		DataDir:          "/var/lib/workload-vmhost",
		Binary:           "/usr/local/bin/firecracker",
		Mode:             ModeSystemd,
		Slice:            "workload-vm.slice",
		NetworkNamespace: "/proc/4242/ns/net",
		FirstUID:         1_000_000_000,
		UIDs:             65536,
		MemoryOverhead:   64 << 20,
	}
}

// spec is a machine as vmhost asks for one: an image and a scratch disk, and
// one network device.
func spec() vm.MachineSpec {
	return vm.MachineSpec{
		ID:        "0123456789abcdef",
		VCPUs:     2,
		CPU:       1.5,
		MemoryMiB: 256,
		Kernel:    "/var/lib/workload-vmhost/boot/vmlinux-0123456789abcdef",
		Initrd:    "/var/lib/workload-vmhost/boot/initrd-0123456789abcdef.cpio.gz",
		Drives: []vm.Drive{
			{Path: "/var/lib/workload-vmhost/images/sha256:abc/rootfs.squashfs", ReadOnly: true},
			{Path: "/var/lib/workload-vmhost/vms/0123456789abcdef/scratch.ext4"},
		},
		NICs: []vm.NIC{{Device: "wkt0123456789ab", MAC: "02:fc:00:00:00:01"}},
		UID:  1_000_000_007,
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()

	t.Run("what vmhost runs machines with is a configuration that works", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, config().check())

		child := config()
		child.Mode, child.Slice, child.UIDs = ModeChild, "", 0
		assert.NoError(t, child.check(), "children need no slice, and may run as vmhost itself")
	})

	t.Run("a configuration no machine could run with is refused", func(t *testing.T) {
		t.Parallel()

		for name, change := range map[string]func(*Config){
			"a relative data directory":                   func(c *Config) { c.DataDir = "var/lib/workload-vmhost" },
			"a data directory that is not clean":          func(c *Config) { c.DataDir = "/var/lib/workload-vmhost/" },
			"no binary":                                   func(c *Config) { c.Binary = "" },
			"fewer than no users":                         func(c *Config) { c.UIDs = -1 },
			"users counting up from root":                 func(c *Config) { c.FirstUID = 0 },
			"users past the last there can be":            func(c *Config) { c.FirstUID = math.MaxInt32 - 10 },
			"a relative network namespace":                func(c *Config) { c.NetworkNamespace = "proc/1/ns/net" },
			"a mode nobody knows":                         func(c *Config) { c.Mode = "jailer" },
			"no slice":                                    func(c *Config) { c.Slice = "" },
			"a slice that is not one":                     func(c *Config) { c.Slice = "workload-vm.service" },
			"a slice that is only its suffix":             func(c *Config) { c.Slice = ".slice" },
			"a data directory in the units' private tmp":  func(c *Config) { c.DataDir = "/tmp/workload-vmhost" },
			"a data directory in a home units cannot see": func(c *Config) { c.DataDir = "/home/vmhost" },
			"a data directory that is root's home":        func(c *Config) { c.DataDir = "/root" },
			"a data directory too long for sockets":       func(c *Config) { c.DataDir = "/var/lib/" + strings.Repeat("workload-vmhost/", 4) },
		} {
			c := config()
			change(&c)

			assert.Error(t, c.check(), name)
		}
	})

	t.Run("a data directory units cannot see is a child's to use", func(t *testing.T) {
		t.Parallel()

		c := config()
		c.Mode, c.DataDir = ModeChild, "/tmp/workload-vmhost"

		assert.NoError(t, c.check())
	})
}

func TestCheckUID(t *testing.T) {
	t.Parallel()

	t.Run("a machine runs as one of the users machines run as", func(t *testing.T) {
		t.Parallel()

		c := config()

		assert.NoError(t, c.checkUID(1_000_000_000), "the first")
		assert.NoError(t, c.checkUID(1_000_065_535), "the last")

		for _, uid := range []int{0, 999_999_999, 1_000_065_536, 1000} {
			assert.ErrorIs(t, c.checkUID(uid), vm.ErrInvalid, uid)
		}
	})

	t.Run("where there are none, every machine runs as vmhost itself", func(t *testing.T) {
		t.Parallel()

		c := config()
		c.UIDs = 0

		assert.NoError(t, c.checkUID(0))
		assert.ErrorIs(t, c.checkUID(1_000_000_000), vm.ErrInvalid, "a machine asking for a user of its own is refused, rather than run as vmhost")
	})
}

func TestCheckSpec(t *testing.T) {
	t.Parallel()

	assert.NoError(t, config().checkSpec(spec()))

	minimal := spec()
	minimal.Initrd, minimal.Drives, minimal.NICs, minimal.CPU = "", nil, nil, 0
	assert.NoError(t, config().checkSpec(minimal), "a machine with nothing but a kernel, all of its CPUs, and no network device")

	for name, change := range map[string]func(*vm.MachineSpec){
		"an ID that is not one":         func(s *vm.MachineSpec) { s.ID = "../../etc" },
		"no CPUs":                       func(s *vm.MachineSpec) { s.VCPUs = 0 },
		"more CPUs than firecracker":    func(s *vm.MachineSpec) { s.VCPUs = 33 },
		"less than none of its CPUs":    func(s *vm.MachineSpec) { s.CPU = -0.5 },
		"a share of its CPUs not there": func(s *vm.MachineSpec) { s.CPU = math.NaN() },
		"no memory":                     func(s *vm.MachineSpec) { s.MemoryMiB = 0 },
		"a relative kernel":             func(s *vm.MachineSpec) { s.Kernel = "vmlinux" },
		"no kernel":                     func(s *vm.MachineSpec) { s.Kernel = "" },
		"a relative initramfs":          func(s *vm.MachineSpec) { s.Initrd = "initrd" },
		"a relative drive":              func(s *vm.MachineSpec) { s.Drives[1].Path = "scratch.ext4" },
		"an unnamed network device":     func(s *vm.MachineSpec) { s.NICs[0].Device = "" },
		"a device name too long":        func(s *vm.MachineSpec) { s.NICs[0].Device = "wkt0123456789abc" },
		"a device name that is a path":  func(s *vm.MachineSpec) { s.NICs[0].Device = "../tap" },
		"a MAC that is not one":         func(s *vm.MachineSpec) { s.NICs[0].MAC = "02:fc:00:00:00" },
		"a MAC that is too long":        func(s *vm.MachineSpec) { s.NICs[0].MAC = "02:fc:00:00:00:00:00:01" },
		"a user out of the range":       func(s *vm.MachineSpec) { s.UID = 1000 },
		"root":                          func(s *vm.MachineSpec) { s.UID = 0 },
	} {
		s := spec()
		change(&s)

		assert.ErrorIs(t, config().checkSpec(s), vm.ErrInvalid, name)
	}
}

func TestMemoryMax(t *testing.T) {
	t.Parallel()

	assert.Equal(t, uint64(256<<20+64<<20), config().memoryMax(spec()), "the guest's memory, and the VMM's overhead beside it")
}

func TestKeyedLock(t *testing.T) {
	t.Parallel()

	var locks keyedLock

	release := locks.hold("0123456789abcdef")

	other := make(chan struct{})
	go func() {
		locks.hold("fedcba9876543210")()
		close(other)
	}()

	select {
	case <-other:
	case <-time.After(5 * time.Second):
		t.Fatal("another machine's lock waited for this one's")
	}

	var held atomic.Bool

	same := make(chan struct{})
	go func() {
		defer close(same)

		locks.hold("0123456789abcdef")()
		held.Store(true)
	}()

	time.Sleep(50 * time.Millisecond)
	assert.False(t, held.Load(), "the same machine's lock is held until it is let go of")

	release()

	select {
	case <-same:
	case <-time.After(5 * time.Second):
		t.Fatal("the lock was never let go of")
	}

	assert.True(t, held.Load())

	locks.lock.Lock()
	defer locks.lock.Unlock()

	require.Empty(t, locks.locks, "a lock nobody holds is forgotten")
}

func TestTakenIDs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	write := func(name string, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		return path
	}

	passwd := write("passwd", "root:x:0:0:root:/root:/bin/bash\n# a comment\nubuntu:x:1000:1000::/home/ubuntu:/bin/bash\nsquatter:x:1000000042:1000:::\n")
	group := write("group", "root:x:0:\nkvm:x:993:\nsquatters:x:1000065535:\n")
	subuid := write("subuid", "ubuntu:524288:1073741824\n")
	subgid := write("subgid", "ubuntu:100000:65536\n")

	assert.Equal(t, []string{
		"squatter in " + passwd + " (1000000042)",
		"squatters in " + group + " (1000065535)",
		"the range ubuntu holds in " + subuid + " (524288-1074266111)",
	}, takenIDs(1_000_000_000, 65536, []string{passwd, group}, []string{subuid, subgid}))

	assert.Empty(t, takenIDs(2_000_000_000, 65536, []string{passwd, group}, []string{subuid, subgid}), "a range nothing holds")
	assert.Empty(t, takenIDs(1_000_000_000, 65536, []string{filepath.Join(dir, "missing")}, nil), "a host that says nothing holds nothing")
}
