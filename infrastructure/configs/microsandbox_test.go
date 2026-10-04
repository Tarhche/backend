package configs

import (
	"io"
	"testing"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkloadMicrosandbox(t *testing.T) {
	t.Run("the plan's defaults", func(t *testing.T) {
		c := NewWorkloadMicrosandbox()

		assert.Equal(t, 8443, c.Port)
		assert.Equal(t, "/data/service", c.StateDir)
		assert.Equal(t, uint64(64<<20), c.MinMemory)
		assert.Zero(t, c.MemoryBudget, "the budget is worked out from the container's limit unless it is given")
		assert.Equal(t, "0.0.0.0", c.PortBindAddress)
		assert.Equal(t, []string{"1.1.1.1", "9.9.9.9"}, c.NameserverList())

		first, last, err := c.HostPorts()
		require.NoError(t, err)
		assert.Equal(t, uint16(20000), first)
		assert.Equal(t, uint16(29999), last)
	})

	t.Run("settings are read from the environment", func(t *testing.T) {
		t.Setenv("SERVER_PORT", "9443")
		t.Setenv("WORKLOAD_MICROSANDBOX_CA_CERT", "ca")
		t.Setenv("WORKLOAD_MICROSANDBOX_CERT", "cert")
		t.Setenv("WORKLOAD_MICROSANDBOX_KEY", "key")
		t.Setenv("WORKLOAD_MICROSANDBOX_STATE_DIR", "/state")
		t.Setenv("WORKLOAD_MICROSANDBOX_MEMORY_BUDGET", "1073741824")
		t.Setenv("WORKLOAD_MICROSANDBOX_MIN_MEMORY", "100663296")
		t.Setenv("WORKLOAD_MICROSANDBOX_PORT_RANGE", "30000-30099")
		t.Setenv("WORKLOAD_MICROSANDBOX_PORT_BIND_ADDRESS", "10.89.0.10")
		t.Setenv("WORKLOAD_MICROSANDBOX_NAMESERVERS", "8.8.8.8")

		c := NewWorkloadMicrosandbox()

		flagSet := console.NewFlagSet("serve-workload-microsandbox", io.Discard)
		require.NoError(t, flagSet.Struct(c))
		require.NoError(t, flagSet.Parse(nil))

		assert.Equal(t, WorkloadMicrosandbox{
			Port:            9443,
			Authority:       "ca",
			Certificate:     "cert",
			Key:             "key",
			StateDir:        "/state",
			MemoryBudget:    1 << 30,
			MinMemory:       96 << 20,
			PortRange:       "30000-30099",
			PortBindAddress: "10.89.0.10",
			Nameservers:     "8.8.8.8",
		}, *c)
	})

	t.Run("the budget is 80% of the container's limit, less 512 MiB", func(t *testing.T) {
		c := NewWorkloadMicrosandbox()

		budget, err := c.Budget(16 << 30)
		require.NoError(t, err)

		assert.Equal(t, uint64(16<<30)/100*80-512<<20, budget)
	})

	t.Run("a budget that is given is the budget", func(t *testing.T) {
		c := NewWorkloadMicrosandbox()
		c.MemoryBudget = 2 << 30

		budget, err := c.Budget(0)
		require.NoError(t, err)

		assert.Equal(t, uint64(2<<30), budget)
	})

	t.Run("a container with no limit and no budget is refused", func(t *testing.T) {
		_, err := NewWorkloadMicrosandbox().Budget(0)

		assert.ErrorContains(t, err, "no memory limit")
	})

	t.Run("a limit that leaves the VMs nothing is refused", func(t *testing.T) {
		_, err := NewWorkloadMicrosandbox().Budget(512 << 20)

		assert.Error(t, err)
	})

	t.Run("a port range that is none is refused", func(t *testing.T) {
		for _, value := range []string{"", "20000", "a-b", "20000-x", "0-10", "20-10", "1-70000"} {
			c := WorkloadMicrosandbox{PortRange: value}

			_, _, err := c.HostPorts()

			assert.Error(t, err, value)
		}

		c := WorkloadMicrosandbox{PortRange: " 20000 - 20000 "}

		first, last, err := c.HostPorts()
		require.NoError(t, err)
		assert.Equal(t, first, last)
	})
}
