//go:build linux

// Package vmm starts the processes microVMs run in, and finds them again.
//
// Each machine is a firecracker process of its own, running in a directory of
// its own as a user of its own: a uid out of a range nothing else uses, and a
// group of the same number. That user owns the machine's directory, the disk
// the machine writes and its taps, and nothing of any other machine's, so a
// machine whose firecracker were ever broken into would hold nothing it did
// not hold already. What a machine boots from is linked into its directory
// rather than copied, since everything is on the one filesystem the state
// directory is on.
//
// None of this needs the host. The launcher runs in a container of its own,
// with a handful of capabilities that reach no further than that container,
// and a machine's process is its child there, sharing /dev/kvm and nothing
// else of the host's.
//
// Nothing is written down about a machine but its directory: which machines
// there are is read off the directories, who each runs as off who owns its
// directory, and which of them run off the processes running as those users.
// So a launcher that restarts finds what it left: a machine whose process is
// gone is one it no longer runs, and is let go when its orchestrator says so.
package vmm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

	// the devices a machine's firecracker opens, which its user has to be
	// let open.
	kvmDevice = "/dev/kvm"
	tunDevice = "/dev/net/tun"

	// commLength is how much of a process's name the kernel keeps.
	commLength = 15
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

	// FirstUID is the first of the users machines run as, and UIDs how many
	// of them there are: each machine is given the lowest no other machine
	// has, and a group of the same number. None at all runs every machine as
	// whoever the launcher runs as, which is for development and tests, and
	// for nothing else.
	FirstUID int
	UIDs     int

	// ClientGID is the orchestrators' group. A machine's directory and the
	// sockets its firecracker makes there are open to it, and to nobody else
	// but the machine.
	ClientGID int
}

// VMM starts, ends and finds machines' firecracker processes.
type VMM struct {
	config Config
	logger *slog.Logger

	// execName is what machines' directories are made under: the name of the
	// firecracker binary they run.
	execName string

	// groups are what a machine's process is given besides its own group:
	// whichever the devices it opens are open to.
	groups []uint32

	lock sync.Mutex
}

var _ machine.VMM = &VMM{}

// New prepares for machines to be started.
func New(config Config, logger *slog.Logger) (*VMM, error) {
	if _, err := os.Stat(config.FirecrackerBinary); err != nil {
		return nil, fmt.Errorf("%s is not there to start machines with: %w", config.FirecrackerBinary, err)
	}

	v := &VMM{config: config, logger: logger, execName: filepath.Base(config.FirecrackerBinary)}

	if v.ownUsers() {
		if config.FirstUID < 1 || int64(config.FirstUID)+int64(config.UIDs)-1 > math.MaxInt32 {
			return nil, fmt.Errorf("machines cannot run as %d users counting up from %d", config.UIDs, config.FirstUID)
		}

		groups, err := deviceGroups(kvmDevice, tunDevice)
		if err != nil {
			return nil, err
		}

		v.groups = groups

		// a machine's firecracker makes its sockets itself, with the mode the
		// umask it started with leaves them, and they are of no use unless the
		// orchestrators' group can open them. So the umask leaves the group
		// everything and everybody else nothing; whatever the launcher makes
		// itself, it gives a mode of its own.
		unix.Umask(0o007)
	}

	// a machine's user goes through these on the way to its own directory,
	// and sees nothing on the way.
	for _, dir := range []string{filepath.Dir(v.baseDir()), v.baseDir()} {
		if err := os.MkdirAll(dir, 0o711); err != nil {
			return nil, err
		}

		if err := os.Chmod(dir, 0o711); err != nil {
			return nil, err
		}
	}

	return v, nil
}

// ownUsers reports whether each machine runs as a user of its own, rather
// than as whoever the launcher runs as.
func (v *VMM) ownUsers() bool {
	return v.config.UIDs > 0
}

// deviceGroups are the groups a machine's process has to be in to open the
// devices it uses: none for a device open to everybody, and the device's own
// for one open to its group.
func deviceGroups(devices ...string) ([]uint32, error) {
	var groups []uint32

	for _, device := range devices {
		var stat unix.Stat_t
		if err := unix.Stat(device, &stat); err != nil {
			return nil, fmt.Errorf("%s is not there for machines to use: %w", device, err)
		}

		switch {
		case stat.Mode&0o006 == 0o006:
		case stat.Mode&0o060 == 0o060:
			if !slices.Contains(groups, stat.Gid) {
				groups = append(groups, stat.Gid)
			}
		default:
			return nil, fmt.Errorf("%s is open to its owner alone, and no machine runs as its owner", device)
		}
	}

	return groups, nil
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

func (v *VMM) Spawn(ctx context.Context, spec machine.Spec) (machine.Machine, error) {
	v.lock.Lock()
	defer v.lock.Unlock()

	// a machine asked for again while it runs is the one that is running.
	if pid, found := v.processes()[spec.ID]; found {
		return v.describe(spec.ID, spec.Owner, pid), nil
	}

	// whatever is left of an earlier machine of the same name goes first, and
	// the user it ran as is free again with it.
	if err := os.RemoveAll(v.machineDir(spec.ID)); err != nil {
		return machine.Machine{}, err
	}

	user, err := v.allocate()
	if err != nil {
		return machine.Machine{}, err
	}

	fail := func(err error) (machine.Machine, error) {
		return machine.Machine{}, errors.Join(err, os.RemoveAll(v.machineDir(spec.ID)))
	}

	if err := v.makeDirectory(spec.ID, user); err != nil {
		return fail(err)
	}

	if err := os.WriteFile(filepath.Join(v.machineDir(spec.ID), ownerFile), []byte(spec.Owner), 0o600); err != nil {
		return fail(err)
	}

	files, err := v.linkFiles(v.rootDir(spec.ID), user, spec.Files)
	if err != nil {
		return fail(err)
	}

	pid, err := v.start(spec, user)
	if err != nil {
		return fail(err)
	}

	launched := v.describe(spec.ID, spec.Owner, pid)
	launched.Files = files

	v.logger.Info("machine started", "machine", spec.ID, "owner", spec.Owner, "pid", pid, "user", user)

	return launched, nil
}

// allocate is the user a machine about to start runs as: the lowest no machine
// holds, since a machine holds the one owning its directory until the
// directory is gone.
func (v *VMM) allocate() (int, error) {
	if !v.ownUsers() {
		return os.Getuid(), nil
	}

	taken := make(map[int]bool)
	for _, id := range v.machineIDs() {
		if user, ok := ownerOf(v.rootDir(id)); ok {
			taken[user] = true
		}
	}

	user, ok := lowestFree(v.config.FirstUID, v.config.UIDs, taken)
	if !ok {
		return 0, fmt.Errorf("all %d users machines run as are taken", v.config.UIDs)
	}

	return user, nil
}

// lowestFree is the lowest of count users from first that is not taken.
func lowestFree(first int, count int, taken map[int]bool) (int, bool) {
	for user := first; user < first+count; user++ {
		if !taken[user] {
			return user, true
		}
	}

	return 0, false
}

// makeDirectory makes a machine's directory, which is the launcher's, and
// which the machine's user goes through without seeing anything; and in it the
// directory the machine runs in, which is its user's and open to the
// orchestrators' group, and to nobody else.
func (v *VMM) makeDirectory(id string, user int) error {
	if err := os.Mkdir(v.machineDir(id), 0o711); err != nil {
		return err
	}

	if err := os.Chmod(v.machineDir(id), 0o711); err != nil {
		return err
	}

	root := v.rootDir(id)

	for _, dir := range []struct {
		path string
		mode os.FileMode
	}{
		{path: root, mode: 0o750},

		// what firecracker makes here — its sockets — takes the directory's
		// group rather than the machine's, which is what lets the
		// orchestrators open them.
		{path: filepath.Join(root, filepath.Dir(socketPath)), mode: 0o770 | os.ModeSetgid},
	} {
		if err := os.Mkdir(dir.path, 0o700); err != nil {
			return err
		}

		// the mode is set while the directory is still the launcher's own,
		// and only then given away: the kernel quietly drops the group bit a
		// process sets on a directory whose group it is not in, and giving a
		// directory away keeps whatever mode it has.
		if err := os.Chmod(dir.path, dir.mode); err != nil {
			return err
		}

		if v.ownUsers() {
			if err := os.Chown(dir.path, user, v.config.ClientGID); err != nil {
				return err
			}
		}
	}

	return nil
}

// start starts a machine's firecracker, as the machine's user, and waits for it
// to take its API.
func (v *VMM) start(spec machine.Spec, user int) (int, error) {
	console, err := os.OpenFile(filepath.Join(v.machineDir(spec.ID), consoleFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	defer console.Close()

	command := exec.Command(v.config.FirecrackerBinary, "--api-sock", socketPath, "--id", spec.ID)
	command.Dir = v.rootDir(spec.ID)
	command.Stdout = console
	command.Stderr = console

	// nothing of the launcher's environment is any business of the machine's.
	command.Env = []string{}

	// the machine is a session of its own, so nothing that happens to the
	// launcher's own is passed on to it.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if v.ownUsers() {
		command.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(user), Gid: uint32(user), Groups: v.groups}
	}

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

// linkFiles links what a machine boots from into its directory, and says what
// its firecracker is to be told they are called.
func (v *VMM) linkFiles(root string, user int, files machine.Files) (machine.Files, error) {
	linked := machine.Files{Kernel: kernelName, Initrd: initrdName}

	if err := v.link(files.Kernel, root, kernelName, user, true); err != nil {
		return machine.Files{}, err
	}

	if err := v.link(files.Initrd, root, initrdName, user, true); err != nil {
		return machine.Files{}, err
	}

	for i, drive := range files.Drives {
		if err := v.link(drive.Path, root, driveName(i), user, drive.ReadOnly); err != nil {
			return machine.Files{}, err
		}

		linked.Drives = append(linked.Drives, machine.Drive{Path: driveName(i), ReadOnly: drive.ReadOnly})
	}

	return linked, nil
}

// link links a file a machine boots from into its directory.
//
// A file has to be under the state directory, has to be a file rather than
// anything pointing elsewhere, and is linked by what it is rather than by its
// name, so nothing swapped in on the way can take it anywhere else, before this
// or while it runs. A file that is only ever read has to be everybody's to read
// already: it may be shared by every machine, and is given to none of them. A
// file the machine writes is made its own, and nobody else's.
func (v *VMM) link(source string, root string, name string, user int, readOnly bool) error {
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
	// a symlink cannot take it anywhere else.
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

	if v.ownUsers() && readOnly && stat.Mode&0o004 == 0 {
		return fmt.Errorf("%s is not everybody's to read, which a file machines share has to be", source)
	}

	// the handle this process holds on the file stands for the file by what
	// it is. Linking the handle itself takes a privilege the launcher does
	// not hold; linking through its name under /proc does not.
	handle := "/proc/self/fd/" + strconv.Itoa(fd)
	target := filepath.Join(root, name)

	if err := unix.Linkat(unix.AT_FDCWD, handle, unix.AT_FDCWD, target, unix.AT_SYMLINK_FOLLOW); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("%s has to be on the same filesystem as %s: %w", source, v.config.StateDir, err)
		}

		return err
	}

	if readOnly || !v.ownUsers() {
		return nil
	}

	if err := unix.Fchownat(unix.AT_FDCWD, handle, user, user, 0); err != nil {
		return err
	}

	return unix.Fchmodat(unix.AT_FDCWD, handle, 0o600, 0)
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

	v.logger.Info("machine terminated", "machine", id)

	return nil
}

func (v *VMM) List(ctx context.Context) ([]machine.Machine, error) {
	v.lock.Lock()
	defer v.lock.Unlock()

	running := v.processes()

	var machines []machine.Machine

	for _, id := range v.machineIDs() {
		owner, err := os.ReadFile(filepath.Join(v.machineDir(id), ownerFile))
		if err != nil {
			continue
		}

		machines = append(machines, v.describe(id, string(owner), running[id]))
	}

	return machines, nil
}

func (v *VMM) describe(id string, owner string, pid int) machine.Machine {
	return machine.Machine{
		ID:      id,
		Owner:   owner,
		PID:     pid,
		Running: pid > 0,
		User:    v.userOf(id),
		Socket:  filepath.Join(v.rootDir(id), socketPath),
		Root:    v.rootDir(id),
	}
}

// userOf is who a machine runs as.
func (v *VMM) userOf(id string) int {
	if !v.ownUsers() {
		return os.Getuid()
	}

	user, _ := ownerOf(v.rootDir(id))

	return user
}

// machineIDs are the machines whose directories there are.
func (v *VMM) machineIDs() []string {
	entries, err := os.ReadDir(v.baseDir())
	if err != nil {
		return nil
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() && machine.IsID(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}

	return ids
}

// processes finds every machine's firecracker that is running.
//
// A machine that runs as a user of its own is found by it: who a process runs
// as is the kernel's to say, and nothing the process can change about itself.
// Run as the launcher itself, as in development, every machine's is found by
// the directory it was started in instead.
func (v *VMM) processes() map[string]int {
	if v.ownUsers() {
		return v.processesByUser()
	}

	return v.processesByDirectory()
}

func (v *VMM) processesByUser() map[string]int {
	found := make(map[string]int)

	machines := make(map[int]string)
	for _, id := range v.machineIDs() {
		if user, ok := ownerOf(v.rootDir(id)); ok {
			machines[user] = id
		}
	}

	if len(machines) == 0 {
		return found
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return found
	}

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		user, ok := realUser(pid)
		if !ok {
			continue
		}

		if id, known := machines[user]; known && v.isFirecracker(pid) {
			found[id] = pid
		}
	}

	return found
}

func (v *VMM) processesByDirectory() map[string]int {
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

// realUser is who a process runs as.
func realUser(pid int) (int, bool) {
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

// isFirecracker reports whether a process is a machine's firecracker that is
// still running. A process is called after the binary it runs, cut short to
// the fifteen characters the kernel keeps.
func (v *VMM) isFirecracker(pid int) bool {
	dir := filepath.Join("/proc", strconv.Itoa(pid))

	comm, err := os.ReadFile(filepath.Join(dir, "comm"))
	if err != nil {
		return false
	}

	name := v.execName
	if len(name) > commLength {
		name = name[:commLength]
	}

	if strings.TrimSuffix(string(comm), "\n") != name {
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

// ownerOf is who owns a file.
func ownerOf(path string) (int, bool) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return 0, false
	}

	return int(stat.Uid), true
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
