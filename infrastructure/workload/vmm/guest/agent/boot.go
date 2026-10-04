//go:build linux

package agent

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// mount is one filesystem the machine needs.
type mount struct {
	source string
	target string
	fstype string
	flags  uintptr
	data   string
}

// the filesystems every machine has before it is told anything: what the
// agent needs to see processes and devices, and somewhere to keep what it
// writes for the task.
var early = []mount{
	{source: "proc", target: "/proc", fstype: "proc", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC},
	{source: "sysfs", target: "/sys", fstype: "sysfs", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC},
	{source: "devtmpfs", target: "/dev", fstype: "devtmpfs", flags: unix.MS_NOSUID, data: "mode=0755"},
	{source: "devpts", target: "/dev/pts", fstype: "devpts", flags: unix.MS_NOSUID | unix.MS_NOEXEC, data: "gid=5,mode=0620,ptmxmode=0666"},
	{source: "tmpfs", target: "/dev/shm", fstype: "tmpfs", flags: unix.MS_NOSUID | unix.MS_NODEV, data: "mode=1777"},
	{source: "tmpfs", target: "/run", fstype: "tmpfs", flags: unix.MS_NOSUID | unix.MS_NODEV, data: "mode=0755"},
}

// cgroupMount is where the machine's cgroups are, which the task is kept in.
var cgroupMount = mount{source: "cgroup2", target: cgroupRoot, fstype: "cgroup2", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC}

// deviceLinks are the links every container finds in its /dev, which the
// kernel's devtmpfs does not make: images take them for granted, nginx's
// sending its logs to /dev/stdout among them. The task's /dev is the agent's,
// so making them here makes them there.
var deviceLinks = map[string]string{
	"/dev/fd":     "/proc/self/fd",
	"/dev/stdin":  "/proc/self/fd/0",
	"/dev/stdout": "/proc/self/fd/1",
	"/dev/stderr": "/proc/self/fd/2",
}

// openFiles is how many files a process in the machine may have open, which
// is what a container is let have and the most the kernel allows by default.
// The kernel starts init with far fewer, and a server the task runs could
// need more.
const openFiles = 1 << 20

// isInit reports whether this process is a machine's init.
func isInit() bool {
	return os.Getpid() == 1
}

// boot makes the machine usable: the kernel starts init with nothing mounted
// but the initramfs it was unpacked from. It says what the task is to be
// kept in: cgroups, or process groups on a kernel that has none.
func boot(logger *slog.Logger) (confinement, error) {
	for _, m := range early {
		if err := mountOne(m); err != nil {
			return nil, err
		}
	}

	for link, target := range deviceLinks {
		if err := os.Symlink(target, link); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}

	// every process started from here on is let have as many files open,
	// since a limit init sets itself is the one its children inherit.
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: openFiles, Max: openFiles}); err != nil {
		logger.Warn("the machine's processes keep the kernel's limit on open files", "error", err)
	}

	// ctrl-alt-del is passed to init, as an interrupt, rather than resetting
	// the machine on the spot: the agent then turns it off in order.
	if err := unix.Reboot(unix.LINUX_REBOOT_CMD_CAD_OFF); err != nil {
		logger.Warn("ctrl-alt-del resets the machine at once", "error", err)
	}

	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return nil, fmt.Errorf("the machine has no loopback: %w", err)
	}

	if err := netlink.LinkSetUp(lo); err != nil {
		return nil, err
	}

	return confinementOf(logger), nil
}

// confinementOf is what the machine can keep the task in. Cgroups are what
// keep everything the task starts together, so they are what it is kept in;
// a kernel without cgroup2 still runs the task, in process groups, which
// something the task starts can leave.
func confinementOf(logger *slog.Logger) confinement {
	if err := mountOne(cgroupMount); err != nil {
		logger.Warn("the machine has no cgroups, so what the task starts is kept in process groups", "error", err)

		return newProcessGroups()
	}

	for _, err := range enableControllers(cgroupRoot) {
		logger.Warn("the task is counted without a controller the kernel has", "error", err)
	}

	return cgroups{root: cgroupRoot}
}

// mountOne mounts m, making its mountpoint first. Something already mounted
// there is taken to be it: the kernel mounts some of these itself.
func mountOne(m mount) error {
	if err := os.MkdirAll(m.target, 0o755); err != nil {
		return err
	}

	err := unix.Mount(m.source, m.target, m.fstype, m.flags, m.data)
	if err != nil && !errors.Is(err, unix.EBUSY) {
		return fmt.Errorf("failed to mount %s on %s: %w", m.fstype, m.target, err)
	}

	return nil
}
