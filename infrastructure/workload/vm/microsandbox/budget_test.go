package microsandbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

func TestBudgetOf(t *testing.T) {
	t.Parallel()

	h := host{CPUs: 6, Memory: 8 << 30, Disk: 60 << 30}

	t.Run("what was given is what is offered", func(t *testing.T) {
		t.Parallel()

		given := vm.Resources{CPUs: 4, Memory: 4 << 30, Disk: 200 << 30}
		assert.Equal(t, given, budgetOf(given, h))
	})

	t.Run("nothing given offers the host's CPUs, 80% of its memory less 512 MiB, and its disk", func(t *testing.T) {
		t.Parallel()

		budget := budgetOf(vm.Resources{}, h)
		assert.Equal(t, uint(6), budget.CPUs)
		assert.Equal(t, uint64(8<<30)/100*80-512<<20, budget.Memory)
		assert.Equal(t, uint64(60<<30), budget.Disk)
	})

	t.Run("a container too small to spare anything offers no memory", func(t *testing.T) {
		t.Parallel()

		assert.Zero(t, budgetOf(vm.Resources{}, host{Memory: 512 << 20}).Memory)
	})
}

func TestFits(t *testing.T) {
	t.Parallel()

	budget := vm.Resources{CPUs: 2, Memory: 4 << 30, Disk: 10 << 30}

	assert.NoError(t, fits(budget, vm.Resources{Memory: 2 << 30, Disk: 5 << 30}, vm.Resources{Memory: 2 << 30, Disk: 5 << 30}), "up to the budget")
	assert.NoError(t, fits(budget, vm.Resources{CPUs: 8}, vm.Resources{CPUs: 8}), "CPUs are given more than once")
	assert.ErrorIs(t, fits(budget, vm.Resources{Memory: 3 << 30}, vm.Resources{Memory: 2 << 30}), vm.ErrNoCapacity)
	assert.ErrorIs(t, fits(budget, vm.Resources{Disk: 6 << 30}, vm.Resources{Disk: 5 << 30}), vm.ErrNoCapacity)
}

// TestComposeVMHostsHoldADockerVM holds every compose file's vmhost, at the
// memory limit it has when nothing says otherwise, to a budget that holds the
// Docker VM made for somebody's first container or stack. With less, that VM
// fits on no node, and every container and stack of anybody without a Docker
// VM fails for want of room.
func TestComposeVMHostsHoldADockerVM(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("../../../../compose*.yaml")
	require.NoError(t, err)

	// the compose files are at the root of the repository, which a test
	// binary run elsewhere, as in the vmhost's image, does not have.
	if len(files) == 0 {
		t.Skip("the compose files are not here to read")
	}

	limit := regexp.MustCompile(`mem_limit: \$\{[A-Z_]*VMHOST_MEMORY_LIMIT:-([0-9]+)([bkmg])\}`)
	units := map[string]uint64{"b": 1, "k": 1 << 10, "m": 1 << 20, "g": 1 << 30}

	dockerVM := configs.NewWorkloadControlPlane().VMDockerDefaultMemory

	vmhosts := 0
	for _, file := range files {
		content, err := os.ReadFile(file)
		require.NoError(t, err)

		for _, match := range limit.FindAllStringSubmatch(string(content), -1) {
			vmhosts++

			size, err := strconv.ParseUint(match[1], 10, 64)
			require.NoError(t, err)

			budget := budgetOf(vm.Resources{}, host{Memory: size * units[match[2]]})
			assert.GreaterOrEqual(t, budget.Memory, dockerVM, "%s: a vmhost limited to %s%s offers %d MiB, and a Docker VM is given %d MiB",
				filepath.Base(file), match[1], match[2], budget.Memory>>20, dockerVM>>20)
		}
	}

	assert.Positive(t, vmhosts, "a vmhost's memory limit is named in the compose files")
}

func TestCgroupMemoryLimit(t *testing.T) {
	t.Parallel()

	limit, err := cgroupMemoryLimit("7516192768\n")
	require.NoError(t, err)
	assert.Equal(t, uint64(7516192768), limit)

	limit, err = cgroupMemoryLimit("max\n")
	require.NoError(t, err)
	assert.Zero(t, limit)

	_, err = cgroupMemoryLimit("lots")
	assert.Error(t, err)
}

func TestMemTotal(t *testing.T) {
	t.Parallel()

	total, err := memTotal("MemTotal:        8110952 kB\nMemFree:         7375888 kB\n")
	require.NoError(t, err)
	assert.Equal(t, uint64(8110952)<<10, total)

	_, err = memTotal("MemFree: 1 kB\n")
	assert.Error(t, err)
}
