package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

func TestHostsFile(t *testing.T) {
	t.Run("a machine answers to its own name, and its neighbours to theirs", func(t *testing.T) {
		content := hostsFile(
			"api-xkfqz",
			[]guest.Interface{{MAC: "06:00:0a:c8:00:02", Address: "10.200.0.2/24"}},
			[]guest.Host{{Address: "10.200.0.3", Names: []string{"db", "db-xkfqz"}}},
		)

		assert.Equal(t, "127.0.0.1\tlocalhost\n"+
			"::1\tlocalhost ip6-localhost ip6-loopback\n"+
			"10.200.0.2\tapi-xkfqz\n"+
			"10.200.0.3\tdb db-xkfqz\n", string(content))
	})

	t.Run("a machine on no network answers to nothing but localhost", func(t *testing.T) {
		assert.Equal(t, "127.0.0.1\tlocalhost\n::1\tlocalhost ip6-localhost ip6-loopback\n", string(hostsFile("job", nil, nil)))
	})
}

func TestResolvConf(t *testing.T) {
	assert.Equal(t, "nameserver 1.1.1.1\nnameserver 8.8.8.8\n", string(resolvConf([]string{"1.1.1.1", "8.8.8.8"})))
	assert.Empty(t, resolvConf(nil), "a machine that cannot reach out is given nowhere to ask")
}

func TestWriteFiles(t *testing.T) {
	t.Run("the files are written where they are, so what is bound to them sees the change", func(t *testing.T) {
		dir := t.TempDir()

		config := guest.Config{Hostname: "api", Nameservers: []string{"1.1.1.1"}}
		require.NoError(t, writeFiles(dir, config))

		before, err := os.Stat(filepath.Join(dir, "hosts"))
		require.NoError(t, err)

		config.Hosts = []guest.Host{{Address: "10.0.0.3", Names: []string{"db"}}}
		require.NoError(t, writeFiles(dir, config))

		after, err := os.Stat(filepath.Join(dir, "hosts"))
		require.NoError(t, err)
		assert.True(t, os.SameFile(before, after), "the file is the same file, rewritten")

		content, err := os.ReadFile(filepath.Join(dir, "hosts"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "10.0.0.3\tdb\n")

		hostname, err := os.ReadFile(filepath.Join(dir, "hostname"))
		require.NoError(t, err)
		assert.Equal(t, "api\n", string(hostname))

		resolv, err := os.ReadFile(filepath.Join(dir, "resolv.conf"))
		require.NoError(t, err)
		assert.Equal(t, "nameserver 1.1.1.1\n", string(resolv))
	})
}

func TestPrimaryAddress(t *testing.T) {
	address, found := primaryAddress([]guest.Interface{{Address: "10.200.0.2/24"}, {Address: "10.201.0.2/24", Gateway: "10.201.0.1"}})

	assert.True(t, found)
	assert.Equal(t, "10.200.0.2", address, "a task is reached on the network it joined first")

	_, found = primaryAddress(nil)
	assert.False(t, found)
}

func TestInRoot(t *testing.T) {
	t.Run("a directory is followed into the root, and made when asked to", func(t *testing.T) {
		root := t.TempDir()

		_, err := directoryInRoot(root, "/proc", false)
		assert.ErrorIs(t, err, os.ErrNotExist)

		dir, err := directoryInRoot(root, "/dev/shm", true)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(root, "dev/shm"), dir)
		assert.DirExists(t, dir)

		same, err := directoryInRoot(root, "/dev/shm", false)
		require.NoError(t, err)
		assert.Equal(t, dir, same)

		top, err := directoryInRoot(root, "/", false)
		require.NoError(t, err)
		assert.Equal(t, root, top)
	})

	t.Run("a link on the way is refused rather than followed out of the root", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()

		require.NoError(t, os.Symlink(outside, filepath.Join(root, "proc")))
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "etc")))

		_, err := directoryInRoot(root, "/proc", true)
		assert.ErrorContains(t, err, "not a directory")

		_, err = fileInRoot(root, "/etc/hosts", true)
		assert.ErrorContains(t, err, "not a directory")

		entries, err := os.ReadDir(outside)
		require.NoError(t, err)
		assert.Empty(t, entries, "nothing was made where the links point")
	})

	t.Run("a file is made when asked to, and a link in its place is replaced, never followed", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "resolv.conf")
		require.NoError(t, os.WriteFile(outside, []byte("the agent's own"), 0o644))

		require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "etc/resolv.conf")))

		_, err := fileInRoot(root, "/etc/resolv.conf", false)
		assert.ErrorContains(t, err, "not a regular file")

		file, err := fileInRoot(root, "/etc/resolv.conf", true)
		require.NoError(t, err)

		info, err := os.Lstat(file)
		require.NoError(t, err)
		assert.True(t, info.Mode().IsRegular())

		content, err := os.ReadFile(outside)
		require.NoError(t, err)
		assert.Equal(t, "the agent's own", string(content), "what the link pointed at is untouched")

		_, err = fileInRoot(root, "/etc/hostname", false)
		assert.ErrorIs(t, err, os.ErrNotExist)

		made, err := fileInRoot(root, "/etc/hostname", true)
		require.NoError(t, err)
		assert.FileExists(t, made)
	})

	t.Run("a path that climbs out is no path inside the root", func(t *testing.T) {
		_, err := directoryInRoot(t.TempDir(), "/../etc", true)
		assert.Error(t, err)
	})
}

func TestStats(t *testing.T) {
	t.Run("CPU is the share of one CPU used between two samples", func(t *testing.T) {
		start := time.Now()

		previous := cpuSample{usage: 1_000_000, at: start}
		current := cpuSample{usage: 1_500_000, at: start.Add(time.Second)}

		assert.InDelta(t, 50.0, cpuPercent(previous, current), 0.001)
		assert.Zero(t, cpuPercent(cpuSample{}, current), "the first sample has nothing to be counted from")
	})

	t.Run("memory is what the machine has, and what of it could still be had", func(t *testing.T) {
		total, available := memoryInfo("MemTotal:         131072 kB\nMemFree:  1024 kB\nMemAvailable:    65536 kB\n")

		assert.Equal(t, uint64(131072*1024), total)
		assert.Equal(t, uint64(65536*1024), available)

		_, available = memoryInfo("MemTotal:         131072 kB\nMemFree:  1024 kB\n")
		assert.Equal(t, uint64(1024*1024), available, "a kernel that does not say what is available says what is free")
	})

	t.Run("network and disks are read the way the kernel writes them", func(t *testing.T) {
		received, sent := networkTotals("Inter-|   Receive  |  Transmit\n" +
			" face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n" +
			"    lo:    100       1    0    0    0     0          0         0      100       1    0    0    0     0       0          0\n" +
			"  eth0:   2000      20    0    0    0     0          0         0     3000      30    0    0    0     0       0          0\n" +
			"  eth1:     10       1    0    0    0     0          0         0       20       1    0    0    0     0       0          0\n")

		assert.Equal(t, uint64(2010), received)
		assert.Equal(t, uint64(3020), sent)

		read, written := diskTotals(" 254       0 vda 100 0 8 50 0 0 0 0 0 0 0\n" +
			" 254      16 vdb 10 0 2 5 4 0 16 3 0 0 0\n" +
			" 254       1 vda1 100 0 8 50 0 0 0 0 0 0 0\n" +
			"   7       0 loop0 1 0 2 0 0 0 0 0 0 0 0\n")

		assert.Equal(t, uint64(10*512), read)
		assert.Equal(t, uint64(16*512), written)
	})
}
