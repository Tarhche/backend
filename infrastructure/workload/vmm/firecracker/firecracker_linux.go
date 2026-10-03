//go:build linux

package firecracker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

const (
	// the devices a machine's firecracker opens, which its user has to be
	// let open.
	kvmDevice = "/dev/kvm"
	tunDevice = "/dev/net/tun"

	// versionTimeout bounds asking the firecracker binary which it is.
	versionTimeout = 5 * time.Second

	// consoleTail is how much of what a machine's firecracker said is kept,
	// from its end, to say why it could not start.
	consoleTail = 2 << 10
)

// New prepares for machines to be booted with Firecracker: it checks what it
// is given, installs the firecracker binary into the data directory by what it
// holds, and, in ModeSystemd, connects to the host's systemd.
//
// Whatever would keep every machine from booting is found here, so that vmhost
// says so when it starts rather than at its first machine: a configuration
// that cannot work, devices a machine's user could not open, a systemd that
// cannot be reached, or a network namespace machines could not join.
func New(config Config, logger *slog.Logger) (*Hypervisor, error) {
	if len(config.Mode) == 0 {
		config.Mode = ModeSystemd
	}

	if err := config.check(); err != nil {
		return nil, err
	}

	if err := layout.Prepare(config.DataDir); err != nil {
		return nil, err
	}

	binary, err := install(config.Binary, layout.Bin(config.DataDir))
	if err != nil {
		return nil, err
	}

	h := &Hypervisor{
		config:   config,
		logger:   logger,
		binary:   binary,
		version:  versionOf(binary),
		execName: execName,
	}

	if len(h.version) == 0 {
		logger.Warn("the firecracker machines run does not say which version it is", "binary", binary)
	}

	if config.UIDs > 0 {
		if taken := takenIDs(config.FirstUID, config.UIDs, userFiles, subordinateFiles); len(taken) > 0 {
			return nil, fmt.Errorf("the %d users machines run as, counting up from %d, are not machines' alone, so whoever holds them could read machines' disks: they are taken by %s", config.UIDs, config.FirstUID, strings.Join(taken, ", "))
		}

		groups, err := deviceGroups(kvmDevice, tunDevice)
		if err != nil {
			return nil, err
		}

		h.groups = groups
	}

	switch config.Mode {
	case ModeChild:
		if err := ownNamespace(config.NetworkNamespace); err != nil {
			return nil, err
		}

		h.launcher = &children{dataDir: config.DataDir, execName: h.execName, byUser: config.UIDs > 0}
	case ModeSystemd:
		namespace, err := unitNamespace(config.NetworkNamespace)
		if err != nil {
			return nil, err
		}

		units, err := newUnits(config.Slice, namespace)
		if err != nil {
			return nil, err
		}

		h.launcher = units
	}

	return h, nil
}

// Version is the version of the firecracker binary machines run.
func (h *Hypervisor) Version() string {
	return h.version
}

// Boot starts a machine's firecracker as the machine's user, where the mode
// says, configures the machine through its API and starts it.
//
// A machine whose firecracker still runs is not booted again: it is
// terminated first, by whoever means to boot it again. Whatever is left of
// one that ended — its directory, a unit not let go of yet — is let go of
// before the machine is made anew. A machine that could not be booted leaves
// nothing behind.
func (h *Hypervisor) Boot(ctx context.Context, spec vm.MachineSpec) (vm.Machine, error) {
	if err := h.config.checkSpec(spec); err != nil {
		return vm.Machine{}, err
	}

	release := h.locks.hold(spec.ID)
	defer release()

	found, err := h.launcher.find(ctx)
	if err != nil {
		return vm.Machine{}, err
	}

	if found[spec.ID].running {
		return vm.Machine{}, fmt.Errorf("%w: machine %s is running already", vm.ErrConflict, spec.ID)
	}

	if err := h.launcher.stop(ctx, spec.ID); err != nil {
		return vm.Machine{}, err
	}

	if err := os.RemoveAll(h.machineDir(spec.ID)); err != nil {
		return vm.Machine{}, err
	}

	fail := func(err error) (vm.Machine, error) {
		// what was made is let go of whatever ctx says, since nothing else
		// knows of it.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*stopTimeout)
		defer cancel()

		return vm.Machine{}, errors.Join(err, h.launcher.stop(cleanup, spec.ID), os.RemoveAll(h.machineDir(spec.ID)))
	}

	if err := h.prepare(spec); err != nil {
		return fail(err)
	}

	started, err := h.launcher.start(ctx, launch{
		id:        spec.ID,
		binary:    filepath.Join(h.machineDir(spec.ID), h.execName),
		dir:       h.rootDir(spec.ID),
		apiSocket: layout.APISocket(h.config.DataDir, spec.ID),
		console:   layout.Console(h.config.DataDir, spec.ID),
		uid:       spec.UID,
		groups:    h.groups,
		cpu:       spec.CPU,
		memoryMax: h.config.memoryMax(spec),
	})
	if err != nil {
		return fail(h.saying(spec.ID, err))
	}

	if err := configure(ctx, layout.APISocket(h.config.DataDir, spec.ID), planOf(spec)); err != nil {
		return fail(h.saying(spec.ID, err))
	}

	h.logger.Info("machine booted", "machine", spec.ID, "pid", started.pid, "user", spec.UID, "unit", started.unit)

	return h.describe(spec.ID, started), nil
}

// Machine is one machine the hypervisor holds: one whose directory is there,
// whether its firecracker runs or not, or whose firecracker runs.
func (h *Hypervisor) Machine(ctx context.Context, id string) (vm.Machine, error) {
	if !vm.IsID(id) {
		return vm.Machine{}, fmt.Errorf("%w: %q is not a machine", vm.ErrNotFound, id)
	}

	found, err := h.launcher.find(ctx)
	if err != nil {
		return vm.Machine{}, err
	}

	p, launched := found[id]
	if !launched && !h.holds(id) {
		return vm.Machine{}, fmt.Errorf("%w: machine %s", vm.ErrNotFound, id)
	}

	return h.describe(id, p), nil
}

// Machines is every machine the hypervisor holds, in the order of their IDs.
func (h *Hypervisor) Machines(ctx context.Context) ([]vm.Machine, error) {
	found, err := h.launcher.find(ctx)
	if err != nil {
		return nil, err
	}

	ids := h.machineIDs()
	for id := range found {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}

	slices.Sort(ids)

	machines := make([]vm.Machine, 0, len(ids))
	for _, id := range ids {
		machines = append(machines, h.describe(id, found[id]))
	}

	return machines, nil
}

// Terminate ends a machine's firecracker, if it still runs, and takes its
// directory away with everything linked into it. What the machine booted from
// stays where it was linked from: its disks are vmhost's to keep or let go of.
func (h *Hypervisor) Terminate(ctx context.Context, id string) error {
	if !vm.IsID(id) {
		return fmt.Errorf("%w: %q is not a machine", vm.ErrInvalid, id)
	}

	release := h.locks.hold(id)
	defer release()

	if err := h.launcher.stop(ctx, id); err != nil {
		return err
	}

	held := h.holds(id)

	if err := os.RemoveAll(h.machineDir(id)); err != nil {
		return err
	}

	if held {
		h.logger.Info("machine terminated", "machine", id)
	}

	return nil
}

// describe is a machine as vmhost is told of it.
func (h *Hypervisor) describe(id string, p process) vm.Machine {
	machine := vm.Machine{
		ID:        id,
		Running:   p.running,
		VsockPath: layout.VsockSocket(h.config.DataDir, id),
		Dir:       h.rootDir(id),
	}

	if p.running {
		machine.PID, machine.Cgroup, machine.Unit = p.pid, p.cgroup, p.unit
	}

	// a machine runs as the user its directory was given to; one that runs
	// as vmhost itself is said to run as nobody of its own, as it was asked.
	if h.config.UIDs > 0 {
		if uid, found := ownerOf(h.rootDir(id)); found {
			machine.UID = uid
		} else if p.running {
			machine.UID, _ = realUser(p.pid)
		}
	}

	return machine
}

// holds reports whether a machine's directory is there.
func (h *Hypervisor) holds(id string) bool {
	info, err := os.Lstat(h.machineDir(id))

	return err == nil && info.IsDir()
}

// machineIDs are the machines whose directories are there.
func (h *Hypervisor) machineIDs() []string {
	return machineIDs(h.config.DataDir)
}

func machineIDs(dataDir string) []string {
	entries, err := os.ReadDir(layout.Machines(dataDir))
	if err != nil {
		return nil
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() && vm.IsID(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}

	return ids
}

func (h *Hypervisor) machineDir(id string) string {
	return layout.Machine(h.config.DataDir, id)
}

func (h *Hypervisor) rootDir(id string) string {
	return layout.MachineRoot(h.config.DataDir, id)
}

// saying adds the end of what a machine's firecracker said to an error, which
// is where firecracker says why it would not do what it was asked.
func (h *Hypervisor) saying(id string, err error) error {
	content, readErr := os.ReadFile(layout.Console(h.config.DataDir, id))
	if readErr != nil || len(content) == 0 {
		return err
	}

	if len(content) > consoleTail {
		content = content[len(content)-consoleTail:]
	}

	return fmt.Errorf("%w; firecracker said: %s", err, strings.TrimSpace(string(content)))
}

// versionOf is which firecracker binary is, as it says itself: "v1.17.0" of
// "Firecracker v1.17.0". One that does not say is of no version anybody knows.
func versionOf(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, binary, "--version").Output()
	if err != nil {
		return ""
	}

	line, _, _ := strings.Cut(string(output), "\n")

	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "Firecracker" {
		return ""
	}

	return fields[1]
}

// ownNamespace reports whether namespace, if one is named, is the network
// namespace vmhost runs in: a child of vmhost runs in vmhost's own, and in no
// other, so machines' taps have to be made there.
func ownNamespace(namespace string) error {
	if len(namespace) == 0 {
		return nil
	}

	var named, own unix.Stat_t

	if err := unix.Stat(namespace, &named); err != nil {
		return fmt.Errorf("the network namespace %s is not there: %w", namespace, err)
	}

	if err := unix.Stat("/proc/self/ns/net", &own); err != nil {
		return err
	}

	if named.Dev != own.Dev || named.Ino != own.Ino {
		return fmt.Errorf("machines started as vmhost's children run in vmhost's own network namespace, and %s is another", namespace)
	}

	return nil
}

// unitNamespace is the network namespace machines' units join, as a path the
// host's systemd opens: the one named, or vmhost's own, named by vmhost's
// process as the host sees it.
//
// A namespace named by a process under /proc is only the one meant if vmhost
// sees the host's processes, which it does when its container shares the
// host's PID namespace: the host's systemd reads the path in the host's /proc,
// where the same number may be another process altogether.
func unitNamespace(namespace string) (string, error) {
	if len(namespace) == 0 {
		namespace = "/proc/" + strconv.Itoa(os.Getpid()) + "/ns/net"
	}

	if strings.HasPrefix(namespace, "/proc/") {
		comm, err := os.ReadFile("/proc/1/comm")
		if err != nil {
			return "", err
		}

		if first := strings.TrimSpace(string(comm)); first != "systemd" {
			return "", fmt.Errorf("the network namespace %s is named by a process, and vmhost does not see the host's processes (the first is %q, not systemd): in %s mode vmhost runs in the host's PID namespace", namespace, first, ModeSystemd)
		}
	}

	if _, err := os.Stat(namespace); err != nil {
		return "", fmt.Errorf("the network namespace %s is not there: %w", namespace, err)
	}

	return namespace, nil
}
