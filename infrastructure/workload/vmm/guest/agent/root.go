//go:build linux

package agent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// Where a task's root is put together. The image is read only and shared by
// every machine that runs it, so a task that may write gets a scratch disk
// laid over it: what it changes lands on the scratch disk, and the image stays
// as it was.
const (
	imageDir   = "/mnt/image"
	scratchDir = "/mnt/scratch"
	rootDir    = "/mnt/root"

	// filesDir keeps the files the agent writes for the task, which are bound
	// over the root's own so that they can change without the root being
	// writable.
	filesDir = "/run/workload"

	// layerOptions is the tmpfs a read-only root is laid over with, which
	// holds nothing but the places things are mounted on.
	layerOptions = "mode=0755,size=4m"
)

// mountRoot puts the task's root together from its disks, and says whether it
// is an overlay the agent can make what it needs in.
//
// A writable root is the image with the scratch disk laid over it. A
// read-only one is the image with a small tmpfs laid over it instead, which
// only the agent writes to: images often lack the places things are mounted
// on — few have an /etc/resolv.conf, since docker makes one for them — and a
// read-only root has nowhere to make them. They are made in the tmpfs, and the
// root is made read only over it before the task runs (finishRoot), so the
// task can change nothing and the image is never written. A kernel that cannot
// lay one over the image gets the image alone, and the task goes without what
// the image has no place for.
func mountRoot(logger *slog.Logger, root guest.Root) (bool, error) {
	for _, dir := range []string{imageDir, scratchDir, rootDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, err
		}
	}

	if err := unix.Mount(root.Image, imageDir, "squashfs", unix.MS_RDONLY, ""); err != nil {
		return false, fmt.Errorf("failed to mount the image: %w", err)
	}

	if root.ReadOnly() {
		if err := unix.Mount("tmpfs", scratchDir, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, layerOptions); err != nil {
			return false, fmt.Errorf("failed to make what a read-only root is laid over with: %w", err)
		}
	} else if err := unix.Mount(root.Scratch, scratchDir, "ext4", 0, ""); err != nil {
		return false, fmt.Errorf("failed to mount the scratch disk: %w", err)
	}

	if err := layer(); err != nil {
		if !root.ReadOnly() {
			return false, fmt.Errorf("failed to lay the scratch disk over the image: %w", err)
		}

		logger.Warn("the read-only root is the image alone, and goes without what it has no place for", "error", err)

		_ = unix.Unmount(scratchDir, 0)

		if err := unix.Mount(imageDir, rootDir, "", unix.MS_BIND, ""); err != nil {
			return false, fmt.Errorf("failed to mount the image as the root: %w", err)
		}

		return false, mountInsideRoot(logger, rootDir, true, false)
	}

	return true, mountInsideRoot(logger, rootDir, root.ReadOnly(), true)
}

// layer lays what is mounted on the scratch directory over the image, as the
// task's root.
func layer() error {
	upper, work := filepath.Join(scratchDir, "upper"), filepath.Join(scratchDir, "work")
	for _, dir := range []string{upper, work} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", imageDir, upper, work)

	return unix.Mount("overlay", rootDir, "overlay", 0, options)
}

// mountInsideRoot gives the task what it expects to find mounted in its own
// root: its processes, the kernel's view of the machine, and its devices; and,
// for a read-only root, somewhere it can still write what is temporary. A
// root the agent cannot make places in goes without a mount it has no place
// for rather than failing: there is nowhere to make one.
func mountInsideRoot(logger *slog.Logger, root string, readOnly bool, create bool) error {
	mounts := []mount{
		{source: "proc", target: "/proc", fstype: "proc", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC},
		{source: "sysfs", target: "/sys", fstype: "sysfs", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_RDONLY},
		{source: "/dev", target: "/dev", flags: unix.MS_BIND | unix.MS_REC},
	}

	if readOnly {
		mounts = append(mounts,
			mount{source: "tmpfs", target: "/tmp", fstype: "tmpfs", flags: unix.MS_NOSUID | unix.MS_NODEV, data: "mode=1777"},
			mount{source: "tmpfs", target: "/run", fstype: "tmpfs", flags: unix.MS_NOSUID | unix.MS_NODEV, data: "mode=0755"},
		)
	}

	for _, m := range mounts {
		target, err := directoryInRoot(root, m.target, create)
		if err != nil {
			logger.Warn("the task's root has nowhere to mount this", "target", m.target, "error", err)

			continue
		}

		if err := unix.Mount(m.source, target, m.fstype, m.flags, m.data); err != nil {
			return fmt.Errorf("failed to mount %s inside the task's root: %w", m.target, err)
		}
	}

	return nil
}

// bindFiles binds the files the agent keeps for the task in files over the
// root's own.
func bindFiles(logger *slog.Logger, files string, root string, create bool) error {
	for name, target := range boundFiles {
		inside, err := fileInRoot(root, target, create)
		if err != nil {
			logger.Warn("the task's root has no place for this file", "file", target, "error", err)

			continue
		}

		if err := unix.Mount(filepath.Join(files, name), inside, "", unix.MS_BIND, ""); err != nil {
			return fmt.Errorf("failed to bind %s into the task's root: %w", target, err)
		}
	}

	return nil
}

// sealRoot makes the root read only, now that everything the task needs has
// been made in it. What is mounted inside it stays as it was.
func sealRoot(root string) error {
	if err := unix.Mount("", root, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("failed to make the task's root read only: %w", err)
	}

	return nil
}

// unmountRoot takes the task's root apart again, as far as it can.
func unmountRoot() {
	for _, dir := range []string{rootDir, scratchDir, imageDir} {
		_ = unix.Unmount(dir, unix.MNT_DETACH)
	}
}
