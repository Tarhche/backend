package vmhost

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// unlimited is past any memory a machine has: cgroup v1 says "no limit" as a
// number near the most an int64 holds.
const unlimited = 1 << 62

// Machine is the Host a vmhost runs on, read off its cgroup, or off the
// machine itself when the cgroup sets no limit.
type Machine struct {
	// root is where the filesystem is read from: "/", or a directory a test
	// laid out like one.
	root string
}

var _ Host = Machine{}

// NewMachine is the host whose filesystem is at root.
func NewMachine(root string) Machine {
	return Machine{root: root}
}

// CPUs are the CPUs this container may use: its CPU limit, rounded up, or
// every CPU the machine has when it has none.
func (m Machine) CPUs() uint {
	if cpus, ok := m.cpuLimit(); ok {
		return cpus
	}

	return uint(runtime.NumCPU())
}

// cpuLimit is the cgroup's CPU limit, as cgroup v2 says it ("quota period"
// in cpu.max, or "max") or cgroup v1 does (cpu.cfs_quota_us, -1 for none).
func (m Machine) cpuLimit() (uint, bool) {
	if fields, err := m.fields("sys/fs/cgroup/cpu.max"); err == nil && len(fields) == 2 && fields[0] != "max" {
		return cpusOf(fields[0], fields[1])
	}

	quota, err := m.fields("sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	if err != nil || len(quota) != 1 {
		return 0, false
	}

	period, err := m.fields("sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if err != nil || len(period) != 1 {
		return 0, false
	}

	return cpusOf(quota[0], period[0])
}

func cpusOf(quota string, period string) (uint, bool) {
	q, err := strconv.ParseFloat(quota, 64)
	if err != nil || q <= 0 {
		return 0, false
	}

	p, err := strconv.ParseFloat(period, 64)
	if err != nil || p <= 0 {
		return 0, false
	}

	return uint(math.Ceil(q / p)), true
}

// Memory is the memory this container may use: its cgroup's limit, as cgroup
// v2 (memory.max) or v1 (memory.limit_in_bytes) says it, or the machine's
// memory when it has none.
func (m Machine) Memory() (uint64, error) {
	for _, file := range []string{"sys/fs/cgroup/memory.max", "sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		fields, err := m.fields(file)
		if err != nil || len(fields) != 1 || fields[0] == "max" {
			continue
		}

		limit, err := strconv.ParseUint(fields[0], 10, 64)
		if err == nil && limit < unlimited {
			return limit, nil
		}
	}

	return m.machineMemory()
}

// machineMemory is MemTotal, which /proc/meminfo says in kB.
func (m Machine) machineMemory() (uint64, error) {
	file, err := os.Open(filepath.Join(m.root, "proc/meminfo"))
	if err != nil {
		return 0, err
	}
	defer file.Close()

	lines := bufio.NewScanner(file)
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}

		kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("MemTotal is not a number: %w", err)
		}

		return kilobytes << 10, nil
	}

	if err := lines.Err(); err != nil {
		return 0, err
	}

	return 0, errors.New("/proc/meminfo has no MemTotal")
}

// fields are the words of a one-line file under root.
func (m Machine) fields(name string) ([]string, error) {
	content, err := os.ReadFile(filepath.Join(m.root, name))
	if err != nil {
		return nil, err
	}

	return strings.Fields(string(content)), nil
}
