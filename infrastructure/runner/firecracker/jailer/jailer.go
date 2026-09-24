//go:build linux

// Package jailer starts the processes microVMs run in, and finds them again.
//
// Each machine is a firecracker process of its own, started through
// firecracker's jailer: chrooted into a directory of its own, running as an
// unprivileged user, in a cgroup of its own that caps what it may use. What a
// machine boots from is linked into that directory rather than copied, since
// everything is on the one filesystem the state directory is on.
//
// Nothing is written down about a machine but its directory: which machines
// there are is read off the directories, and which of them run is read off the
// cgroups the jailer puts them in, which are named after them. So the launcher
// can restart, or be replaced, while its machines keep running, and find them
// where it left them.
package jailer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/layout"
)

const (
	// socketPath is where a machine's firecracker takes its API, inside its
	// own directory.
	socketPath = "run/firecracker.socket"

	// ownerFile and consoleFile are kept beside a machine's directory rather
	// than in it, where the machine cannot reach them.
	ownerFile   = "owner"
	consoleFile = "console.log"

	// startTimeout is how long a machine's firecracker has to be ready to be
	// configured, and killTimeout how long it has to be gone once killed.
	startTimeout = 10 * time.Second
	killTimeout  = 10 * time.Second

	// memoryOverheadMiB is what a machine's firecracker needs beyond the
	// memory it gives the machine.
	memoryOverheadMiB = 128

	// cpuPeriod is the period a machine's CPU quota is counted over, in
	// microseconds.
	cpuPeriod = 100_000

	cgroupRoot = "/sys/fs/cgroup"
)

// the names a machine's boot files are given inside its directory.
const (
	kernelName = "vmlinux"
	initrdName = "initrd"
)

func driveName(index int) string {
	return "drive" + strconv.Itoa(index)
}

// Config is how machines' processes are started.
type Config struct {
	// StateDir is the directory everything a machine boots from is under.
	// Machines' own directories are made in it too, so what they boot from
	// can be linked rather than copied.
	StateDir string

	FirecrackerBinary string

	// JailerBinary starts each firecracker jailed. Without it firecracker is
	// started as it is, as whoever the launcher runs as, with nothing between
	// a machine and the host but the machine itself: that is for development,
	// and for tests, and for nothing else.
	JailerBinary string

	// UID and GID are who a jailed firecracker runs as. What it boots from
	// has to be theirs to read, and its drives theirs to write.
	UID int
	GID int
}

// VMM starts, ends and finds machines' firecracker processes.
type VMM struct {
	config Config
	logger *slog.Logger

	// execName is what the jailer names machines' directories after: the
	// name of the firecracker binary it runs. It names the cgroup it puts
	// them under too.
	execName string

	// cgroups is where the host's cgroups are, which is only ever somewhere
	// else in tests.
	cgroups string

	lock sync.Mutex
}

var _ machine.VMM = &VMM{}

// New prepares for machines to be started.
func New(config Config, logger *slog.Logger) (*VMM, error) {
	binaries := []string{config.FirecrackerBinary}
	if len(config.JailerBinary) > 0 {
		binaries = append(binaries, config.JailerBinary)
	}

	for _, binary := range binaries {
		if _, err := os.Stat(binary); err != nil {
			return nil, fmt.Errorf("%s is not there to start machines with: %w", binary, err)
		}
	}

	v := &VMM{config: config, logger: logger, execName: filepath.Base(config.FirecrackerBinary), cgroups: cgroupRoot}

	if err := os.MkdirAll(v.baseDir(), 0o755); err != nil {
		return nil, err
	}

	// the jailer makes the devices a machine uses inside its directory, and a
	// device on a filesystem mounted nodev cannot be opened: every machine
	// would fail as its firecracker reached for one, which is better said now.
	if v.jailed() {
		nodev, err := mountedNodev(v.baseDir())
		if err != nil {
			return nil, err
		}

		if nodev {
			return nil, fmt.Errorf("%s is on a filesystem mounted nodev, where the devices a jailed machine uses cannot be opened", config.StateDir)
		}
	}

	return v, nil
}

// mountedNodev reports whether the filesystem path is on is mounted nodev.
func mountedNodev(path string) (bool, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return false, err
	}

	return stat.Flags&unix.ST_NODEV != 0, nil
}

func (v *VMM) baseDir() string {
	return filepath.Join(v.config.StateDir, layout.JailDir, v.execName)
}

func (v *VMM) machineDir(id string) string {
	return filepath.Join(v.baseDir(), id)
}

func (v *VMM) rootDir(id string) string {
	return filepath.Join(v.machineDir(id), "root")
}

// cgroupDir is the cgroup the jailer makes each machine's own cgroup in, which
// it names after the binary it runs, as it does their directories.
func (v *VMM) cgroupDir() string {
	return filepath.Join(v.cgroups, v.execName)
}

func (v *VMM) Spawn(ctx context.Context, spec machine.Spec, taps []machine.AttachedTap) (machine.Machine, error) {
	v.lock.Lock()
	defer v.lock.Unlock()

	// a machine asked for again while it runs is the one that is running.
	if pid, found := v.processes()[spec.ID]; found {
		return v.describe(spec.ID, spec.Owner, pid, taps), nil
	}

	// whatever is left of an earlier machine of the same name goes first.
	if err := os.RemoveAll(v.machineDir(spec.ID)); err != nil {
		return machine.Machine{}, err
	}

	root := v.rootDir(spec.ID)
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(socketPath)), 0o755); err != nil {
		return machine.Machine{}, err
	}

	if err := os.Chown(filepath.Join(root, filepath.Dir(socketPath)), v.config.UID, v.config.GID); err != nil && v.jailed() {
		return machine.Machine{}, err
	}

	if err := os.WriteFile(filepath.Join(v.machineDir(spec.ID), ownerFile), []byte(spec.Owner), 0o644); err != nil {
		return machine.Machine{}, err
	}

	files, err := v.linkFiles(root, spec.Files)
	if err != nil {
		return machine.Machine{}, errors.Join(err, os.RemoveAll(v.machineDir(spec.ID)))
	}

	pid, err := v.start(spec)
	if err != nil {
		return machine.Machine{}, errors.Join(err, os.RemoveAll(v.machineDir(spec.ID)))
	}

	launched := v.describe(spec.ID, spec.Owner, pid, taps)
	launched.Files = files

	v.logger.Info("machine started", "machine", spec.ID, "owner", spec.Owner, "pid", pid, "jailed", v.jailed())

	return launched, nil
}

// start starts a machine's firecracker and waits for it to take its API.
func (v *VMM) start(spec machine.Spec) (int, error) {
	console, err := os.OpenFile(filepath.Join(v.machineDir(spec.ID), consoleFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return 0, err
	}
	defer console.Close()

	command := v.command(spec)
	command.Stdout = console
	command.Stderr = console

	// the machine is a session of its own, so nothing that happens to the
	// launcher's own is passed on to it.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("failed to start the machine's firecracker: %w", err)
	}

	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()

	socket := filepath.Join(v.rootDir(spec.ID), socketPath)
	deadline := time.Now().Add(startTimeout)

	for {
		if _, err := os.Stat(socket); err == nil {
			return command.Process.Pid, nil
		}

		select {
		case err := <-exited:
			return 0, fmt.Errorf("the machine's firecracker ended before it was ready (%v): %s", err, v.consoleTail(spec.ID))
		case <-time.After(10 * time.Millisecond):
		}

		if time.Now().After(deadline) {
			_ = command.Process.Kill()

			return 0, fmt.Errorf("the machine's firecracker was not ready after %s: %s", startTimeout, v.consoleTail(spec.ID))
		}
	}
}

// command is how a machine's firecracker is started: through the jailer when
// there is one, and as it is when there is not.
func (v *VMM) command(spec machine.Spec) *exec.Cmd {
	if !v.jailed() {
		command := exec.Command(v.config.FirecrackerBinary, "--api-sock", socketPath, "--id", spec.ID)
		command.Dir = v.rootDir(spec.ID)

		return command
	}

	args := []string{
		"--id", spec.ID,
		"--exec-file", v.config.FirecrackerBinary,
		"--uid", strconv.Itoa(v.config.UID),
		"--gid", strconv.Itoa(v.config.GID),
		"--chroot-base-dir", filepath.Join(v.config.StateDir, layout.JailDir),
		"--cgroup-version", "2",
	}

	if spec.CPUQuota > 0 {
		args = append(args, "--cgroup", fmt.Sprintf("cpu.max=%d %d", int(spec.CPUQuota*cpuPeriod), cpuPeriod))
	}

	// a host without the memory controller still runs machines: each is
	// held to its own memory by what it was given, and only firecracker's
	// own is left uncounted.
	if controllerAvailable("memory") {
		args = append(args, "--cgroup", fmt.Sprintf("memory.max=%d", (spec.MemoryMiB+memoryOverheadMiB)<<20))
	}

	args = append(args, "--", "--api-sock", "/"+socketPath)

	return exec.Command(v.config.JailerBinary, args...)
}

func (v *VMM) jailed() bool {
	return len(v.config.JailerBinary) > 0
}

// linkFiles links what a machine boots from into its directory, and says what
// its firecracker is to be told they are called.
//
// A file has to be under the state directory, has to be a file rather than
// anything pointing elsewhere, and has to be one the machine can read: the
// launcher never changes whose a file is, so nothing asked of it can hand a
// machine a file of the host's.
func (v *VMM) linkFiles(root string, files machine.Files) (machine.Files, error) {
	linked := machine.Files{Kernel: kernelName, Initrd: initrdName}

	if err := v.link(files.Kernel, root, kernelName); err != nil {
		return machine.Files{}, err
	}

	if err := v.link(files.Initrd, root, initrdName); err != nil {
		return machine.Files{}, err
	}

	for i, drive := range files.Drives {
		if err := v.link(drive, root, driveName(i)); err != nil {
			return machine.Files{}, err
		}

		linked.Drives = append(linked.Drives, driveName(i))
	}

	return linked, nil
}

func (v *VMM) link(source string, root string, name string) error {
	if !filepath.IsAbs(source) {
		return fmt.Errorf("%q is not a path the launcher takes", source)
	}

	relative, err := filepath.Rel(v.config.StateDir, filepath.Clean(source))
	if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
		return fmt.Errorf("%s is not under %s, where machines boot from", source, v.config.StateDir)
	}

	state, err := os.Open(v.config.StateDir)
	if err != nil {
		return err
	}
	defer state.Close()

	// the file is found from the state directory down, through nothing that
	// is a symlink and never above where it started: a directory swapped for
	// a symlink cannot take it anywhere else, before this or while it runs.
	fd, err := unix.Openat2(int(state.Fd()), relative, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return fmt.Errorf("%s is not a file a machine can boot from: %w", source, err)
	}
	defer unix.Close(fd)

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}

	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("%s is not a file", source)
	}

	if v.jailed() && int(stat.Uid) != v.config.UID && stat.Mode&0o004 == 0 {
		return fmt.Errorf("%s is not the machine's to read", source)
	}

	target := filepath.Join(root, name)

	// what is linked is the file that was checked, by what it is rather than
	// by its name. That takes a privilege only the jailing launcher has; one
	// started as anybody links by name, which is what it can do.
	if v.jailed() {
		err = unix.Linkat(fd, "", unix.AT_FDCWD, target, unix.AT_EMPTY_PATH)
	} else {
		err = os.Link(filepath.Join(v.config.StateDir, relative), target)
	}

	if errors.Is(err, syscall.EXDEV) {
		return fmt.Errorf("%s has to be on the same filesystem as %s: %w", source, v.config.StateDir, err)
	}

	return err
}

func (v *VMM) Kill(ctx context.Context, id string) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	if pid, found := v.processes()[id]; found {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}

		deadline := time.Now().Add(killTimeout)
		for {
			if _, running := v.processes()[id]; !running {
				break
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("machine %s is still running after %s", id, killTimeout)
			}

			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := os.RemoveAll(v.machineDir(id)); err != nil {
		return err
	}

	// the jailer's cgroup for the machine is empty now, and goes with it.
	_ = os.Remove(filepath.Join(v.cgroupDir(), id))

	v.logger.Info("machine terminated", "machine", id)

	return nil
}

func (v *VMM) List(ctx context.Context) ([]machine.Machine, error) {
	v.lock.Lock()
	defer v.lock.Unlock()

	entries, err := os.ReadDir(v.baseDir())
	if err != nil {
		return nil, err
	}

	running := v.processes()

	machines := make([]machine.Machine, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !machine.IsID(entry.Name()) {
			continue
		}

		owner, err := os.ReadFile(filepath.Join(v.baseDir(), entry.Name(), ownerFile))
		if err != nil {
			continue
		}

		machines = append(machines, v.describe(entry.Name(), string(owner), running[entry.Name()], nil))
	}

	return machines, nil
}

func (v *VMM) describe(id string, owner string, pid int, taps []machine.AttachedTap) machine.Machine {
	return machine.Machine{
		ID:      id,
		Owner:   owner,
		PID:     pid,
		Running: pid > 0,
		Socket:  filepath.Join(v.rootDir(id), socketPath),
		Root:    v.rootDir(id),
		Taps:    taps,
	}
}

// processes finds every machine's firecracker that is running.
//
// A jailed one is found by its cgroup: the jailer moves each machine's
// firecracker into a cgroup of its own, named after the machine, before it
// becomes firecracker, and nothing inside the jail can leave it. Where it runs
// would not say which machine it is: the jailer makes the machine's directory
// the root of a mount namespace of its own, and from outside that its working
// directory reads as "/". One that is not jailed runs where it was started,
// which is its machine's directory, and is found by that.
func (v *VMM) processes() map[string]int {
	if v.jailed() {
		return v.jailedProcesses()
	}

	return v.startedProcesses()
}

func (v *VMM) jailedProcesses() map[string]int {
	found := make(map[string]int)

	entries, err := os.ReadDir(v.cgroupDir())
	if err != nil {
		return found
	}

	for _, entry := range entries {
		if !entry.IsDir() || !machine.IsID(entry.Name()) {
			continue
		}

		procs, err := os.ReadFile(filepath.Join(v.cgroupDir(), entry.Name(), "cgroup.procs"))
		if err != nil {
			continue
		}

		for field := range strings.FieldsSeq(string(procs)) {
			pid, err := strconv.Atoi(field)
			if err == nil && v.isFirecracker(pid) {
				found[entry.Name()] = pid

				break
			}
		}
	}

	return found
}

func (v *VMM) startedProcesses() map[string]int {
	found := make(map[string]int)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return found
	}

	prefix := v.baseDir() + string(filepath.Separator)

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !v.isFirecracker(pid) {
			continue
		}

		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err != nil || !strings.HasPrefix(cwd, prefix) {
			continue
		}

		id, _, _ := strings.Cut(strings.TrimPrefix(cwd, prefix), string(filepath.Separator))

		if machine.IsID(id) {
			found[id] = pid
		}
	}

	return found
}

// isFirecracker reports whether a process is a firecracker that is still
// running.
func (v *VMM) isFirecracker(pid int) bool {
	dir := filepath.Join("/proc", strconv.Itoa(pid))

	comm, err := os.ReadFile(filepath.Join(dir, "comm"))
	if err != nil || !strings.HasPrefix(string(comm), "firecracker") {
		return false
	}

	return !isZombie(dir)
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

// consoleTail is the end of what a machine's firecracker said, for when it
// said why it could not start.
func (v *VMM) consoleTail(id string) string {
	content, err := os.ReadFile(filepath.Join(v.machineDir(id), consoleFile))
	if err != nil {
		return ""
	}

	const tail = 2 << 10
	if len(content) > tail {
		content = content[len(content)-tail:]
	}

	return strings.TrimSpace(string(content))
}

// controllerAvailable reports whether the host's cgroups have a controller.
func controllerAvailable(controller string) bool {
	controllers, err := os.ReadFile(filepath.Join(cgroupRoot, "cgroup.controllers"))
	if err != nil {
		return false
	}

	for _, c := range strings.Fields(string(controllers)) {
		if c == controller {
			return true
		}
	}

	return false
}
