//go:build linux

package agent

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A task is everything that runs in it, not only the process it was started
// as, so what it runs is kept in a cgroup of its own. That is what lets it be
// counted, and ended, as a whole: nothing it starts can leave the cgroup,
// because a process is started inside it rather than moved in afterwards.
//
//	/sys/fs/cgroup/task          the task, counted and ended as one
//	/sys/fs/cgroup/task/main     the process it was started as, and its children
//	/sys/fs/cgroup/task/exec-ID  a command run in it alongside, and its children
const (
	cgroupRoot = "/sys/fs/cgroup"

	// emptyInterval is how often a cgroup that is being emptied is looked
	// at again.
	emptyInterval = 10 * time.Millisecond
)

// the controllers a task is counted with. A kernel without one of them counts
// the task without it.
var controllers = []string{"cpu", "memory", "pids", "io"}

// cgroups keeps the task in cgroups under root.
type cgroups struct {
	root string
}

var _ confinement = cgroups{}

func (c cgroups) task() string {
	return filepath.Join(c.root, "task")
}

// dir is the cgroup of the group called name, and the task's own for none.
func (c cgroups) dir(name string) string {
	if len(name) == 0 {
		return c.task()
	}

	return filepath.Join(c.task(), name)
}

func (c cgroups) prepare(name string) (*os.File, error) {
	return openCgroup(c.task(), c.dir(name))
}

func (c cgroups) started(string, int) {}

func (c cgroups) populated(name string) (bool, error) {
	return isPopulated(c.dir(name))
}

func (c cgroups) signal(name string, signal syscall.Signal) (bool, error) {
	return signalCgroup(c.dir(name), signal)
}

func (c cgroups) kill(name string, timeout time.Duration) error {
	return killCgroup(c.dir(name), timeout)
}

func (c cgroups) remove(name string, timeout time.Duration) error {
	return removeCgroup(c.dir(name), timeout)
}

func (c cgroups) usage() usage {
	return usage{
		cpu:  readKeyed(filepath.Join(c.task(), "cpu.stat"), "usage_usec"),
		pids: readCounter(filepath.Join(c.task(), "pids.current")),
	}
}

// enableControllers hands the controllers a cgroup has on to its children,
// one at a time, so that one the kernel will not hand on costs the task that
// controller and no other. It says which it could not.
func enableControllers(dir string) []error {
	available, err := os.ReadFile(filepath.Join(dir, "cgroup.controllers"))
	if err != nil {
		return []error{err}
	}

	var failed []error
	for _, controller := range strings.Fields(string(available)) {
		if !slices.Contains(controllers, controller) {
			continue
		}

		if err := os.WriteFile(filepath.Join(dir, "cgroup.subtree_control"), []byte("+"+controller), 0o644); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", controller, err))
		}
	}

	return failed
}

// openCgroup makes a cgroup, and the task's one above it, and opens it for a
// process to be started inside.
func openCgroup(task string, dir string) (*os.File, error) {
	if _, err := os.Stat(task); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(task, 0o755); err != nil {
			return nil, err
		}

		if failed := enableControllers(task); len(failed) > 0 {
			return nil, errors.Join(failed...)
		}

		limitMemory(task, readText("/proc/meminfo"))
	}

	if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	return os.Open(dir)
}

// taskMemoryReserve is what the agent keeps for itself when it holds the task
// to the machine's memory: its log ring, the buffers of the commands and
// connections it carries, and room to answer while the task is being ended.
const taskMemoryReserve = 8 << 20

// limitMemory holds the task to what the machine has left once it is up,
// short of what the agent keeps for itself.
//
// A machine is as big as its task asked for, so a task that takes more takes
// the whole machine with it. The kernel then takes back every page it can
// before it ends anything, and the pages a machine runs from are read again
// from its compressed image every time they are wanted, which on a busy host
// goes on for minutes while the task neither runs nor ends. A cgroup at its
// limit is ended at once instead, as a container is at its own.
//
// It is the most the task can have, not what it is promised. A kernel without
// the memory controller runs the task as before.
func limitMemory(task string, meminfo string) {
	_, available := memoryInfo(meminfo)
	if available <= taskMemoryReserve {
		return
	}

	_ = os.WriteFile(filepath.Join(task, "memory.max"), []byte(strconv.FormatUint(available-taskMemoryReserve, 10)), 0o644)
}

// killCgroup ends everything in a cgroup at once, and waits for it to be
// gone.
func killCgroup(dir string, timeout time.Duration) error {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err := os.WriteFile(filepath.Join(dir, "cgroup.kill"), []byte("1"), 0o644); err != nil {
		return err
	}

	return waitEmpty(dir, timeout)
}

// removeCgroup ends everything in a cgroup and takes the cgroup away, and
// every cgroup below it.
func removeCgroup(dir string, timeout time.Duration) error {
	if err := killCgroup(dir, timeout); err != nil {
		return err
	}

	children, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	for _, child := range children {
		if child.IsDir() {
			if err := removeCgroup(filepath.Join(dir, child.Name()), timeout); err != nil {
				return err
			}
		}
	}

	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// waitEmpty waits until nothing runs in a cgroup any more, or timeout passes.
func waitEmpty(dir string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		populated, err := isPopulated(dir)
		if err != nil || !populated {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%s still holds processes after %s", dir, timeout)
		}

		time.Sleep(emptyInterval)
	}
}

// isPopulated reports whether anything runs in a cgroup, or below it.
func isPopulated(dir string) (bool, error) {
	events, err := os.Open(filepath.Join(dir, "cgroup.events"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}
	defer events.Close()

	scanner := bufio.NewScanner(events)
	for scanner.Scan() {
		if key, value, found := strings.Cut(scanner.Text(), " "); found && key == "populated" {
			return value == "1", nil
		}
	}

	return false, scanner.Err()
}

// signalCgroup sends a signal to every process in a cgroup and below it, and
// reports whether there was any to send it to.
func signalCgroup(dir string, signal syscall.Signal) (bool, error) {
	pids, err := cgroupProcesses(dir)
	if err != nil {
		return false, err
	}

	signalled := false
	for _, pid := range pids {
		if err := syscall.Kill(pid, signal); err == nil {
			signalled = true
		}
	}

	return signalled, nil
}

// cgroupProcesses is every process in a cgroup and below it.
func cgroupProcesses(dir string) ([]int, error) {
	var pids []int

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}

			return err
		}

		if entry.IsDir() || entry.Name() != "cgroup.procs" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		for _, field := range strings.Fields(string(content)) {
			if pid, err := strconv.Atoi(field); err == nil {
				pids = append(pids, pid)
			}
		}

		return nil
	})

	return pids, err
}

// readCounter reads a cgroup file holding one number.
func readCounter(path string) uint64 {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	value, _ := strconv.ParseUint(strings.TrimSpace(string(content)), 10, 64)

	return value
}

// readKeyed reads one key of a cgroup file of "key value" lines.
func readKeyed(path string, key string) uint64 {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(content), "\n") {
		if name, value, found := strings.Cut(line, " "); found && name == key {
			parsed, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)

			return parsed
		}
	}

	return 0
}
