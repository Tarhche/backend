//go:build linux

package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
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

func TestPrimaryAddress(t *testing.T) {
	address, found := primaryAddress([]guest.Interface{{Address: "10.200.0.2/24"}, {Address: "10.201.0.2/24", Gateway: "10.201.0.1"}})

	assert.True(t, found)
	assert.Equal(t, "10.200.0.2", address, "a task is reached on the network it joined first")

	_, found = primaryAddress(nil)
	assert.False(t, found)
}

func TestStats(t *testing.T) {
	t.Run("CPU is the share of one CPU used between two samples", func(t *testing.T) {
		start := time.Now()

		previous := cpuSample{usage: 1_000_000, at: start}
		current := cpuSample{usage: 1_500_000, at: start.Add(time.Second)}

		assert.InDelta(t, 50.0, cpuPercent(previous, current), 0.001)
		assert.Zero(t, cpuPercent(cpuSample{}, current), "the first sample has nothing to be counted from")
	})

	t.Run("memory, network and disks are read the way the kernel writes them", func(t *testing.T) {
		assert.Equal(t, uint64(131072*1024), memoryTotal("MemTotal:         131072 kB\nMemFree:  1 kB\n"))

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
