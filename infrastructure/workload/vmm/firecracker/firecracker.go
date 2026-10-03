// Package firecracker is vmhost's Hypervisor for Firecracker.
//
// It starts a machine's firecracker as the machine's own host user, links
// what the machine boots from into the machine's own directory, configures
// the machine over its API socket with firecracker-go-sdk — its size, its
// kernel and initramfs, its drives, its network devices and its vsock — and
// boots it. A machine's user owns its directory, its scratch disk and its
// taps, and nothing of any other machine's; firecracker's own seccomp filters
// stay on.
//
// Where a machine's firecracker runs is the Mode. ModeSystemd starts it as a
// transient unit of the host's systemd, over D-Bus, in the slice every machine
// shares: it is then the host's process rather than vmhost's, and outlives
// vmhost's container, which is redeployed with every release. ModeChild starts
// it as vmhost's own child, which ends with vmhost, and is for development.
// Either way the machine runs in the network namespace its taps are in, and a
// vmhost that restarts finds its machines again by their IDs alone.
//
// A machine is laid out on vmhost's data directory (vmm/layout) as
//
//	j/<id>/               vmhost's: the machine's user goes through it and sees nothing
//	j/<id>/firecracker    the VMM, linked from bin/ by what it holds
//	j/<id>/console.log    what the VMM and the guest's kernel said, out of the machine's reach
//	j/<id>/root/          the machine's user's own, where its VMM runs
//	j/<id>/root/vmlinux   the kernel, and initrd the initramfs, linked from boot/
//	j/<id>/root/drive<n>  its disks, in the order it finds them
//	j/<id>/root/run/      the sockets its VMM makes: its API, and its vsock
//
// Everything is linked rather than copied, by what it is rather than by its
// name, from under the data directory and through nothing that is a symlink.
// What is only read — the VMM, the kernel, the initramfs, an image — is shared
// by every machine and has to be everybody's to read already; a disk the
// machine writes is made its user's alone. The VMM is linked beside the
// machine's own directory rather than into it, where its user could replace
// it, and it is reached through these links rather than through bin/ and
// boot/, which are vmhost's alone. Being linked, the VMM a machine was started
// from stays whole however often vmhost is upgraded.
//
// Nothing is written down about a machine but its directory, which is how the
// machines there are are known: the directory is the machine, from the moment
// it is booted until it is terminated, whether its VMM still runs or not. Which
// ones run is asked of where they run: in ModeSystemd, the unit named after
// the machine (workload-vm-<id>.service) and its main process; in ModeChild,
// the processes called firecracker that run as a machine's user, or, when
// every machine runs as vmhost itself, in a machine's directory.
//
// This is PR #101's launcher, which started machines as children of its own
// container, moved into vmhost and given the units mode, so that machines
// outlive vmhost.
package firecracker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// Mode is where a machine's firecracker runs.
type Mode string

const (
	// ModeSystemd runs it as a transient host systemd unit, which outlives
	// vmhost.
	ModeSystemd Mode = "systemd"

	// ModeChild runs it as vmhost's own child, which ends with vmhost.
	ModeChild Mode = "child"
)

// Config is how machines' firecrackers are started.
type Config struct {
	// DataDir is vmhost's data directory (infrastructure/workload/vmm/layout),
	// which is the same path on the host: a unit runs what it is given there.
	DataDir string

	// Binary is the firecracker binary, which is copied into the data
	// directory by what it holds before anything is run from it, so an
	// upgrade never replaces one a machine was started from.
	Binary string

	// Mode is where machines' firecrackers run. Empty is ModeSystemd.
	Mode Mode

	// Slice is the systemd slice machines' units run in.
	Slice string

	// NetworkNamespace is the network namespace machines' firecrackers run in,
	// as a path the host's systemd can open. Empty is vmhost's own.
	NetworkNamespace string

	// FirstUID and UIDs are the host users machines run as: the range a
	// machine's user has to be in, and the one a machine left running is
	// found by. UIDs zero runs every machine as vmhost itself, which is for
	// development and nothing else.
	FirstUID int
	UIDs     int

	// MemoryOverhead is what a machine's firecracker is let use beside its
	// guest's memory, in bytes, before its unit is held to it.
	MemoryOverhead uint64
}

// name is what this hypervisor is called.
const name = "firecracker"

const (
	// execName is what a machine's firecracker is called beside its
	// directory, and so what the kernel calls its process.
	execName = "firecracker"

	// kernelName and initrdName are what a machine's kernel and initramfs are
	// called in its own directory, where its firecracker is told to find
	// them; its drives are drive0, drive1 and on, and its network devices
	// eth0, eth1 and on.
	kernelName = "vmlinux"
	initrdName = "initrd"

	// maxVCPUs is the most CPUs firecracker gives one machine.
	maxVCPUs = 32

	// maxDeviceName is the longest a network device's name can be.
	maxDeviceName = 15

	// maxSocketPath is the longest path a unix socket can be reached by on
	// linux: its address has room for 108 bytes, the last of them the
	// terminator.
	maxSocketPath = 107

	// startTimeout is how long a machine's firecracker has to take its API
	// once it is started, and stopTimeout how long it has to be gone once it
	// is ended.
	startTimeout = 10 * time.Second
	stopTimeout  = 10 * time.Second
)

func driveName(index int) string {
	return "drive" + strconv.Itoa(index)
}

func nicName(index int) string {
	return "eth" + strconv.Itoa(index)
}

// Hypervisor boots machines with Firecracker.
type Hypervisor struct {
	config Config
	logger *slog.Logger

	// binary is the firecracker machines are started from, installed in the
	// data directory by what it holds, and version what it says it is.
	binary  string
	version string

	// execName is what a machine's firecracker is linked as, and so what its
	// process is called. Tests change it, so that a real firecracker running
	// beside them is never taken for one of theirs.
	execName string

	// groups are what a machine's user is given besides its own group: the
	// ones the devices its firecracker opens are open to.
	groups []uint32

	// launcher is where machines' firecrackers run.
	launcher launcher

	locks keyedLock
}

var _ vm.Hypervisor = (*Hypervisor)(nil)

// Name is the VMM this hypervisor boots machines with.
func (h *Hypervisor) Name() string {
	return name
}

// Close lets go of what the hypervisor holds open, which is its connection to
// the host's systemd. Machines carry on as they were.
func (h *Hypervisor) Close() error {
	if h == nil || h.launcher == nil {
		return nil
	}

	return h.launcher.close()
}

// hiddenFromUnits are where a unit sees something else than vmhost does: its
// own /tmp (PrivateTmp=), and no homes at all (ProtectHome=). A data directory
// under one of them would be somewhere else for a machine's firecracker.
var hiddenFromUnits = []string{"/tmp", "/var/tmp", "/home", "/root", "/run/user"}

// check reports what is wrong with the configuration, if anything: what is
// wrong with it is wrong for every machine, so it is found before any is
// booted.
func (c Config) check() error {
	if !filepath.IsAbs(c.DataDir) || filepath.Clean(c.DataDir) != c.DataDir {
		return fmt.Errorf("machines are made in a data directory given as a clean absolute path, not %q", c.DataDir)
	}

	// a machine's firecracker makes its sockets by paths relative to where it
	// runs, which are short; vmhost reaches them by their whole paths, which
	// a unix socket's address has room for 107 bytes of.
	if longest := layout.APISocket(c.DataDir, strings.Repeat("f", 16)); len(longest) > maxSocketPath {
		return fmt.Errorf("the data directory %s is too long for machines' sockets to be reached in it: %s is longer than %d bytes", c.DataDir, longest, maxSocketPath)
	}

	if len(c.Binary) == 0 {
		return fmt.Errorf("no firecracker binary is given to start machines with")
	}

	if c.UIDs < 0 {
		return fmt.Errorf("machines cannot run as %d users", c.UIDs)
	}

	if c.UIDs > 0 && (c.FirstUID < 1 || int64(c.FirstUID)+int64(c.UIDs)-1 > math.MaxInt32) {
		return fmt.Errorf("machines cannot run as %d users counting up from %d", c.UIDs, c.FirstUID)
	}

	if len(c.NetworkNamespace) > 0 && !filepath.IsAbs(c.NetworkNamespace) {
		return fmt.Errorf("the network namespace machines run in is given as an absolute path, not %q", c.NetworkNamespace)
	}

	switch c.Mode {
	case ModeChild:
		return nil
	case ModeSystemd:
	default:
		return fmt.Errorf("machines cannot run in mode %q: it is %q or %q", c.Mode, ModeSystemd, ModeChild)
	}

	if !strings.HasSuffix(c.Slice, ".slice") || len(c.Slice) == len(".slice") || strings.Contains(c.Slice, "/") {
		return fmt.Errorf("machines' units run in a slice, not in %q", c.Slice)
	}

	for _, hidden := range hiddenFromUnits {
		if c.DataDir == hidden || strings.HasPrefix(c.DataDir, hidden+"/") {
			return fmt.Errorf("the data directory %s is under %s, which machines' units do not see as vmhost does", c.DataDir, hidden)
		}
	}

	return nil
}

// checkUID reports whether a machine can run as uid: as one of the users
// machines run as, or as vmhost itself where there are none.
func (c Config) checkUID(uid int) error {
	if c.UIDs == 0 {
		if uid != 0 {
			return fmt.Errorf("%w: machines run as vmhost itself here, not as user %d", vm.ErrInvalid, uid)
		}

		return nil
	}

	if uid < c.FirstUID || uid >= c.FirstUID+c.UIDs {
		return fmt.Errorf("%w: user %d is not one of the %d machines run as, counting up from %d", vm.ErrInvalid, uid, c.UIDs, c.FirstUID)
	}

	return nil
}

// checkSpec reports what is wrong with a machine before anything is made for
// it: what firecracker would refuse, and what this hypervisor will not do.
// Whether the files it boots from are ones it may boot from is found when they
// are linked.
func (c Config) checkSpec(spec vm.MachineSpec) error {
	if !vm.IsID(spec.ID) {
		return fmt.Errorf("%w: %q is not a machine", vm.ErrInvalid, spec.ID)
	}

	if spec.VCPUs < 1 || spec.VCPUs > maxVCPUs {
		return fmt.Errorf("%w: a machine has 1 to %d CPUs, not %d", vm.ErrInvalid, maxVCPUs, spec.VCPUs)
	}

	if spec.CPU < 0 || math.IsNaN(spec.CPU) || math.IsInf(spec.CPU, 0) {
		return fmt.Errorf("%w: a machine cannot keep %v of its CPUs busy", vm.ErrInvalid, spec.CPU)
	}

	if spec.MemoryMiB < 1 {
		return fmt.Errorf("%w: a machine cannot have %d MiB of memory", vm.ErrInvalid, spec.MemoryMiB)
	}

	if !filepath.IsAbs(spec.Kernel) {
		return fmt.Errorf("%w: a machine boots a kernel given as an absolute path, not %q", vm.ErrInvalid, spec.Kernel)
	}

	if len(spec.Initrd) > 0 && !filepath.IsAbs(spec.Initrd) {
		return fmt.Errorf("%w: a machine boots an initramfs given as an absolute path, not %q", vm.ErrInvalid, spec.Initrd)
	}

	for i, drive := range spec.Drives {
		if !filepath.IsAbs(drive.Path) {
			return fmt.Errorf("%w: drive %d is not given as an absolute path, but as %q", vm.ErrInvalid, i, drive.Path)
		}
	}

	for i, nic := range spec.NICs {
		if len(nic.Device) == 0 || len(nic.Device) > maxDeviceName || strings.ContainsAny(nic.Device, "/ ") {
			return fmt.Errorf("%w: network device %d cannot be called %q", vm.ErrInvalid, i, nic.Device)
		}

		if mac, err := net.ParseMAC(nic.MAC); err != nil || len(mac) != 6 {
			return fmt.Errorf("%w: network device %d cannot be found by %q, which is not a MAC", vm.ErrInvalid, i, nic.MAC)
		}
	}

	return c.checkUID(spec.UID)
}

// memoryMax is the most memory a machine's firecracker may use: its guest's,
// and what the VMM uses beside it.
func (c Config) memoryMax(spec vm.MachineSpec) uint64 {
	return uint64(spec.MemoryMiB)<<20 + c.MemoryOverhead
}

// keyedLock holds one lock for each machine, so that booting one machine never
// waits for another, and booting and terminating the same one never meet.
type keyedLock struct {
	lock  sync.Mutex
	locks map[string]*heldLock
}

type heldLock struct {
	sync.Mutex
	holders int
}

// hold takes id's lock, and hands back what lets go of it.
func (k *keyedLock) hold(id string) func() {
	k.lock.Lock()

	if k.locks == nil {
		k.locks = make(map[string]*heldLock)
	}

	held, found := k.locks[id]
	if !found {
		held = &heldLock{}
		k.locks[id] = held
	}

	held.holders++
	k.lock.Unlock()

	held.Lock()

	return func() {
		held.Unlock()

		k.lock.Lock()
		defer k.lock.Unlock()

		held.holders--
		if held.holders == 0 {
			delete(k.locks, id)
		}
	}
}

// launcher is where machines' firecrackers run, and what finds them there
// again: host units, or vmhost's children.
type launcher interface {
	// start starts a machine's firecracker as l says, and hands it back once
	// it has taken its API socket.
	start(ctx context.Context, l launch) (process, error)

	// find is every machine's firecracker the launcher holds, by machine. A
	// machine whose firecracker ended may or may not be among them, as not
	// running; one that is not among them is not running either.
	find(ctx context.Context) (map[string]process, error)

	// stop ends a machine's firecracker, and waits until it is gone. One that
	// is not running is the outcome asked for.
	stop(ctx context.Context, id string) error

	// close lets go of what the launcher holds open.
	close() error
}

// launch is a machine's firecracker to start.
type launch struct {
	id string

	// binary is the firecracker to run, as linked beside the machine's
	// directory, and dir that directory, where it runs.
	binary string
	dir    string

	// apiSocket is where it takes its API, and console where what it says
	// goes.
	apiSocket string
	console   string

	// uid is who it runs as, with a group of the same number and groups
	// besides; zero is vmhost itself.
	uid    int
	groups []uint32

	// cpu is the share of its CPUs it may keep busy, zero being all of them,
	// and memoryMax the most memory it may use, in bytes.
	cpu       float64
	memoryMax uint64
}

// process is a machine's firecracker as a launcher finds it.
type process struct {
	running bool
	pid     int

	// unit is the host unit running it, and cgroup its own cgroup, when it
	// has them.
	unit   string
	cgroup string
}
