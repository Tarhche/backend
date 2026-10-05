package microsandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
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
