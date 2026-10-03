//go:build linux

package firecracker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

const (
	// commLength is how much of a process's name the kernel keeps.
	commLength = 15

	// pollInterval is how often a firecracker is looked at again while it is
	// waited for.
	pollInterval = 10 * time.Millisecond
)

// children starts machines' firecrackers as vmhost's own children, which is
// for development: they are in vmhost's cgroup and its network namespace, are
// held to nothing of their own, and end when vmhost's container does.
//
// Nothing is kept about them but their directories. A machine that runs as a
// user of its own is found by its process running as that user, which is the
// kernel's to say and nothing the process can change about itself; one that
// runs as vmhost itself is found by the directory its process was started in.
// So a vmhost that is started again in the same container finds the children
// its earlier self left running, as #101's launcher did.
type children struct {
	dataDir string

	// execName is what machines' firecrackers are called.
	execName string

	// byUser says machines run as users of their own, and are found by them.
	byUser bool
}

var _ launcher = (*children)(nil)

func (c *children) start(ctx context.Context, l launch) (process, error) {
	console, err := os.OpenFile(l.console, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return process{}, err
	}
	defer console.Close()

	// the firecracker outlives the request that started it, so it is not
	// bound to ctx.
	command := exec.Command(l.binary, "--api-sock", layout.APISocketName, "--id", l.id)
	command.Dir = l.dir
	command.Stdout = console
	command.Stderr = console

	// nothing of vmhost's environment is any business of the machine's.
	command.Env = []string{}

	// the machine is a session of its own, so nothing that happens to
	// vmhost's own is passed on to it.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if l.uid > 0 {
		command.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(l.uid), Gid: uint32(l.uid), Groups: l.groups}
	}

	if err := command.Start(); err != nil {
		return process{}, fmt.Errorf("the machine's firecracker did not start: %w", err)
	}

	// it is collected whenever it ends, by vmhost, whose child it is.
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()

	ended := func() error {
		select {
		case err := <-exited:
			return fmt.Errorf("the machine's firecracker ended before it was ready (%v)", err)
		default:
			return nil
		}
	}

	if err := awaitAPI(ctx, l.apiSocket, ended); err != nil {
		_ = command.Process.Kill()

		return process{}, err
	}

	return process{running: true, pid: command.Process.Pid}, nil
}

func (c *children) find(ctx context.Context) (map[string]process, error) {
	found := make(map[string]process)

	for id, pid := range c.processes() {
		found[id] = process{running: true, pid: pid}
	}

	return found, nil
}

func (c *children) stop(ctx context.Context, id string) error {
	pid, found := c.processes()[id]
	if !found {
		return nil
	}

	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}

	deadline := time.Now().Add(stopTimeout)

	for {
		if _, running := c.processes()[id]; !running {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("machine %s is still running %s after it was killed", id, stopTimeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (c *children) close() error {
	return nil
}

// processes finds every machine's firecracker that is running, by machine.
func (c *children) processes() map[string]int {
	if c.byUser {
		return c.processesByUser()
	}

	return c.processesByDirectory()
}

// processesByUser finds machines' firecrackers by who they run as: a machine
// runs as the user its directory was given to.
func (c *children) processesByUser() map[string]int {
	found := make(map[string]int)

	machines := make(map[int]string)
	for _, id := range machineIDs(c.dataDir) {
		if user, ok := ownerOf(layout.MachineRoot(c.dataDir, id)); ok {
			machines[user] = id
		}
	}

	if len(machines) == 0 {
		return found
	}

	for _, pid := range pids() {
		user, ok := realUser(pid)
		if !ok {
			continue
		}

		if id, known := machines[user]; known && c.isFirecracker(pid) {
			found[id] = pid
		}
	}

	return found
}

// processesByDirectory finds machines' firecrackers by where they run: in a
// machine's own directory.
func (c *children) processesByDirectory() map[string]int {
	found := make(map[string]int)

	prefix := layout.Machines(c.dataDir) + string(filepath.Separator)

	for _, pid := range pids() {
		if !c.isFirecracker(pid) {
			continue
		}

		cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
		if err != nil || !strings.HasPrefix(cwd, prefix) {
			continue
		}

		id, _, _ := strings.Cut(strings.TrimPrefix(cwd, prefix), string(filepath.Separator))

		if vm.IsID(id) {
			found[id] = pid
		}
	}

	return found
}

// isFirecracker reports whether a process is a machine's firecracker that is
// still running. A process is called after the binary it runs, cut short to
// the fifteen characters the kernel keeps.
func (c *children) isFirecracker(pid int) bool {
	dir := filepath.Join("/proc", strconv.Itoa(pid))

	comm, err := os.ReadFile(filepath.Join(dir, "comm"))
	if err != nil {
		return false
	}

	name := c.execName
	if len(name) > commLength {
		name = name[:commLength]
	}

	if strings.TrimSuffix(string(comm), "\n") != name {
		return false
	}

	return !isZombie(dir)
}

// pids are the processes there are.
func pids() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	var found []int
	for _, entry := range entries {
		if pid, err := strconv.Atoi(entry.Name()); err == nil {
			found = append(found, pid)
		}
	}

	return found
}

// realUser is who a process runs as.
func realUser(pid int) (int, bool) {
	if pid <= 0 {
		return 0, false
	}

	status, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, false
	}

	for line := range strings.Lines(string(status)) {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				return 0, false
			}

			user, err := strconv.Atoi(fields[0])

			return user, err == nil
		}
	}

	return 0, false
}

// isZombie reports whether a process has ended and is waiting to be collected.
func isZombie(dir string) bool {
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return true
	}

	// the state follows the command, which is in brackets and may itself
	// hold anything.
	closing := bytes.LastIndexByte(stat, ')')
	if closing < 0 || closing+2 >= len(stat) {
		return true
	}

	return stat[closing+2] == 'Z'
}

// awaitAPI waits for a machine's firecracker to take its API on socket, which
// is when it can be told what the machine is: for as long as ended says it has
// not, and no longer than startTimeout. It is taken once a connection to it is
// answered, rather than once it is there, since firecracker makes the socket
// moments before it listens on it.
func awaitAPI(ctx context.Context, socket string, ended func() error) error {
	deadline := time.Now().Add(startTimeout)

	for {
		if conn, err := net.DialTimeout("unix", socket, pollInterval*10); err == nil {
			conn.Close()

			return nil
		}

		if err := ended(); err != nil {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("the machine's firecracker did not take its API within %s", startTimeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
