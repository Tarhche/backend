//go:build linux

package agent

import (
	"log/slog"
	"time"

	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// machine is what the agent does to the machine it is init of, as against
// what it does for the task in it: these change the machine itself, and only
// init may.
type machine interface {
	// setClock sets the machine's clock to now.
	setClock(now time.Time) error

	// mountRoot puts the task's root together from the machine's disks,
	// with what the task expects to find mounted inside it.
	mountRoot(root guest.Root) error

	// configureInterfaces gives the machine's network devices their
	// addresses, and its default route.
	configureInterfaces(interfaces []guest.Interface) error

	// setHostname gives the machine its name.
	setHostname(hostname string) error

	// finishRoot binds the files the agent keeps for the task into its
	// root, and makes a read-only root read only, now that nothing more has
	// to be made in it.
	finishRoot(readOnly bool) error

	// powerOff puts the machine's disks away and turns it off. It does not
	// return when it works.
	powerOff()
}

// vm is the machine the agent runs in.
type vm struct {
	logger *slog.Logger

	// layered says the task's root is an overlay the agent could make what
	// it needed in, which a read-only one is made read only over once it
	// has.
	layered bool
}

var _ machine = (*vm)(nil)

func (m *vm) setClock(now time.Time) error {
	timeval := unix.NsecToTimeval(now.UnixNano())

	return unix.Settimeofday(&timeval)
}

func (m *vm) mountRoot(root guest.Root) error {
	layered, err := mountRoot(m.logger, root)
	m.layered = layered

	return err
}

func (m *vm) configureInterfaces(interfaces []guest.Interface) error {
	return configureInterfaces(interfaces)
}

func (m *vm) setHostname(hostname string) error {
	return unix.Sethostname([]byte(hostname))
}

func (m *vm) finishRoot(readOnly bool) error {
	if err := bindFiles(m.logger, filesDir, rootDir, m.layered); err != nil {
		return err
	}

	if readOnly && m.layered {
		return sealRoot(rootDir)
	}

	return nil
}

func (m *vm) powerOff() {
	unix.Sync()
	unmountRoot()
	unix.Sync()

	// firecracker has no power button: a reset is what ends it.
	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
}

// Halt turns the machine off at once, for when there is nothing left to run it
// for. It does nothing in anything but a machine's init.
func Halt() {
	if !isInit() {
		return
	}

	unix.Sync()
	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
}
