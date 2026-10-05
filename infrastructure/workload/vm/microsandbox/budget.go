package microsandbox

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	mib = 1 << 20

	// reservedMemory is what of the container's memory is never offered to
	// VMs: the vmhost's own, and the SDK's beside it.
	reservedMemory = 512 * mib

	// offeredShare is the part of the container's memory, in percent, that is
	// offered at most. Each VM costs its host a little more than its guest is
	// given, and the page cache wants some too.
	offeredShare = 80
)

// host is what the budget is worked out from where the options give none.
type host struct {
	// CPUs is how many CPUs the vmhost may run on.
	CPUs int

	// Memory is the container's memory limit, or the host's memory when the
	// container has none.
	Memory uint64

	// Disk is the size of the filesystem the engine keeps VMs on.
	Disk uint64
}

// budgetOf is what a node offers to VMs: what it was given, and for anything
// it was given none of, what the host has. Memory is 80% of the container's
// limit, less what the vmhost keeps for itself.
func budgetOf(given vm.Resources, h host) vm.Resources {
	budget := given

	if budget.CPUs == 0 {
		budget.CPUs = uint(max(h.CPUs, 1))
	}

	if budget.Memory == 0 {
		budget.Memory = max(h.Memory/100*offeredShare, reservedMemory) - reservedMemory
	}

	if budget.Disk == 0 {
		budget.Disk = h.Disk
	}

	return budget
}

// fits refuses what would take a node's memory or disk past its budget, given
// what the other instances it holds were given. CPUs are never refused: a
// node's CPUs are shared rather than given away, and the control plane gives
// them more than once on purpose.
func fits(budget vm.Resources, others vm.Resources, asked vm.Resources) error {
	if others.Memory+asked.Memory > budget.Memory {
		return fmt.Errorf("%w: %d bytes of memory asked, %d of %d given", vm.ErrNoCapacity, asked.Memory, others.Memory, budget.Memory)
	}

	if others.Disk+asked.Disk > budget.Disk {
		return fmt.Errorf("%w: %d bytes of disk asked, %d of %d given", vm.ErrNoCapacity, asked.Disk, others.Disk, budget.Disk)
	}

	return nil
}

// hostOf is what the vmhost's container has: the CPUs it may run on, its
// memory limit or the host's memory when it has none, and the size of the
// filesystem home is on.
func hostOf(home string) host {
	h := host{CPUs: runtime.NumCPU()}

	if meminfo, err := os.ReadFile("/proc/meminfo"); err == nil {
		h.Memory, _ = memTotal(string(meminfo))
	}

	if content, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		if limit, err := cgroupMemoryLimit(string(content)); err == nil && limit > 0 && (h.Memory == 0 || limit < h.Memory) {
			h.Memory = limit
		}
	}

	var fs syscall.Statfs_t
	if err := syscall.Statfs(home, &fs); err == nil {
		h.Disk = uint64(fs.Blocks) * uint64(fs.Bsize)
	}

	return h
}

// cgroupMemoryLimit reads a cgroup v2 memory.max: a number of bytes, or "max"
// for none, which reads as zero.
func cgroupMemoryLimit(content string) (uint64, error) {
	content = strings.TrimSpace(content)
	if content == "max" {
		return 0, nil
	}

	return strconv.ParseUint(content, 10, 64)
}

// memTotal reads the host's memory, in bytes, out of /proc/meminfo.
func memTotal(meminfo string) (uint64, error) {
	scanner := bufio.NewScanner(strings.NewReader(meminfo))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}

		kib, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}

		return kib << 10, nil
	}

	return 0, fmt.Errorf("no MemTotal in /proc/meminfo")
}
