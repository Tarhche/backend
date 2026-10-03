package firecracker

import (
	"fmt"
	"os"
	"strings"
	"testing"

	systemd "github.com/coreos/go-systemd/v22/dbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnitName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "workload-vm-0123456789abcdef.service", unitName("0123456789abcdef"))

	id, ok := machineOfUnit("workload-vm-0123456789abcdef.service")
	assert.True(t, ok)
	assert.Equal(t, "0123456789abcdef", id)

	for _, unit := range []string{
		"workload-vm.slice",
		"workload-vm-0123456789abcdef.scope",
		"workload-vm-not-a-machine.service",
		"docker.service",
	} {
		_, ok := machineOfUnit(unit)
		assert.False(t, ok, unit)
	}
}

// render writes unit properties out the way D-Bus carries them: each name, its
// type and its value. D-Bus's own text form has no way to write a command
// (ExecStart's structs), which is written as Go writes it instead.
func render(properties []systemd.Property) string {
	var rendered strings.Builder

	for _, p := range properties {
		value := p.Value.String()
		if strings.Contains(value, "INVALID") {
			value = fmt.Sprintf("%+v", p.Value.Value())
		}

		fmt.Fprintf(&rendered, "%s %s %s\n", p.Name, p.Value.Signature(), value)
	}

	return rendered.String()
}

func TestUnitProperties(t *testing.T) {
	t.Parallel()

	t.Run("a machine's unit runs it as its own user, held to its size, and sees nothing but its own directory", func(t *testing.T) {
		t.Parallel()

		l := launch{
			id:        "0123456789abcdef",
			binary:    "/var/lib/workload-vmhost/j/0123456789abcdef/firecracker",
			dir:       "/var/lib/workload-vmhost/j/0123456789abcdef/root",
			apiSocket: "/var/lib/workload-vmhost/j/0123456789abcdef/root/run/firecracker.socket",
			console:   "/var/lib/workload-vmhost/j/0123456789abcdef/console.log",
			uid:       1_000_000_007,
			groups:    []uint32{993},
			cpu:       1.5,
			memoryMax: config().memoryMax(spec()),
		}

		expected, err := os.ReadFile("testdata/unit-properties.txt")
		require.NoError(t, err)

		assert.Equal(t, string(expected), render(unitProperties(l, "workload-vm.slice", "/proc/4242/ns/net")))
	})

	t.Run("a machine that runs as vmhost, on all its CPUs, in vmhost's namespace, is given none of them", func(t *testing.T) {
		t.Parallel()

		rendered := render(unitProperties(launch{id: "0123456789abcdef", binary: "/var/lib/workload-vmhost/j/0123456789abcdef/firecracker", memoryMax: 1 << 30}, "workload-vm.slice", ""))

		for _, absent := range []string{"CapabilityBoundingSet ", "CPUQuotaPerSecUSec ", "NetworkNamespacePath "} {
			assert.NotContains(t, rendered, "\n"+absent, absent)
		}

		assert.NotContains(t, rendered, setpriv, "it runs firecracker itself, as root")
		assert.Contains(t, rendered, "\nExecStart a(sasb) [{Path:/var/lib/workload-vmhost/j/0123456789abcdef/firecracker ")
		assert.Contains(t, rendered, "\nMemoryMax t @t 1073741824\n")
	})

	t.Run("a machine's user with no groups besides its own is given none", func(t *testing.T) {
		t.Parallel()

		rendered := render(unitProperties(launch{id: "0123456789abcdef", binary: "/var/lib/workload-vmhost/j/0123456789abcdef/firecracker", uid: 1_000_000_007}, "workload-vm.slice", ""))

		assert.Contains(t, rendered, " --clear-groups ")
		assert.NotContains(t, rendered, "--groups=")
	})
}

func TestUnitCgroup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/sys/fs/cgroup/workload.slice/workload-vm.slice/workload-vm-0123456789abcdef.service", unitCgroup("/workload.slice/workload-vm.slice/workload-vm-0123456789abcdef.service"))
	assert.Empty(t, unitCgroup(""), "a unit with no cgroup of its own has none")
}
