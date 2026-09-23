//go:build linux

package agent

import (
	"errors"
	"fmt"
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
// agent needs to see processes, devices and cgroups, and somewhere to keep
// what it writes for the task.
var early = []mount{
	{source: "proc", target: "/proc", fstype: "proc", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC},
	{source: "sysfs", target: "/sys", fstype: "sysfs", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC},
	{source: "devtmpfs", target: "/dev", fstype: "devtmpfs", flags: unix.MS_NOSUID, data: "mode=0755"},
	{source: "devpts", target: "/dev/pts", fstype: "devpts", flags: unix.MS_NOSUID | unix.MS_NOEXEC, data: "gid=5,mode=0620,ptmxmode=0666"},
	{source: "tmpfs", target: "/dev/shm", fstype: "tmpfs", flags: unix.MS_NOSUID | unix.MS_NODEV, data: "mode=1777"},
	{source: "tmpfs", target: "/run", fstype: "tmpfs", flags: unix.MS_NOSUID | unix.MS_NODEV, data: "mode=0755"},
	{source: "cgroup2", target: cgroupRoot, fstype: "cgroup2", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC},
}

// boot makes the machine usable: the kernel starts init with nothing mounted
// but the initramfs it was unpacked from.
func boot() error {
	for _, m := range early {
		if err := mountOne(m); err != nil {
			return err
		}
	}

	if err := enableControllers(cgroupRoot); err != nil {
		return err
	}

	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("the machine has no loopback: %w", err)
	}

	return netlink.LinkSetUp(lo)
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
