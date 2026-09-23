//go:build linux

package agent

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
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
	filesDir = "/run/runner"
)

// the files the agent keeps for the task, and where they appear inside it.
var boundFiles = map[string]string{
	"hosts":       "/etc/hosts",
	"resolv.conf": "/etc/resolv.conf",
	"hostname":    "/etc/hostname",
}

// mountRoot puts the task's root together from its disks.
func mountRoot(root guest.Root) error {
	for _, dir := range []string{imageDir, scratchDir, rootDir, filesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	if root.ReadOnly() {
		if err := unix.Mount(root.Image, rootDir, "ext4", unix.MS_RDONLY, ""); err != nil {
			return fmt.Errorf("failed to mount the image: %w", err)
		}
	} else {
		if err := unix.Mount(root.Image, imageDir, "ext4", unix.MS_RDONLY, ""); err != nil {
			return fmt.Errorf("failed to mount the image: %w", err)
		}

		if err := unix.Mount(root.Scratch, scratchDir, "ext4", 0, ""); err != nil {
			return fmt.Errorf("failed to mount the scratch disk: %w", err)
		}

		upper, work := filepath.Join(scratchDir, "upper"), filepath.Join(scratchDir, "work")
		for _, dir := range []string{upper, work} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}

		options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", imageDir, upper, work)
		if err := unix.Mount("overlay", rootDir, "overlay", 0, options); err != nil {
			return fmt.Errorf("failed to lay the scratch disk over the image: %w", err)
		}
	}

	return mountInsideRoot(!root.ReadOnly())
}

// mountInsideRoot gives the task what it expects to find mounted in its own
// root. A read-only root that lacks a mountpoint goes without that mount
// rather than failing: there is nowhere to make one.
func mountInsideRoot(writable bool) error {
	mounts := []mount{
		{source: "proc", target: "/proc", fstype: "proc", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC},
		{source: "sysfs", target: "/sys", fstype: "sysfs", flags: unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_RDONLY},
		{source: "/dev", target: "/dev", flags: unix.MS_BIND | unix.MS_REC},
	}

	for _, m := range mounts {
		target := filepath.Join(rootDir, m.target)

		if writable {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		} else if _, err := os.Stat(target); err != nil {
			slog.Warn("the task's root has nowhere to mount this", "target", m.target)

			continue
		}

		if err := unix.Mount(m.source, target, m.fstype, m.flags, m.data); err != nil {
			return fmt.Errorf("failed to mount %s inside the task's root: %w", m.target, err)
		}
	}

	return nil
}

// bindFiles binds the files the agent keeps for the task over the root's own.
func bindFiles(writable bool) error {
	for name, target := range boundFiles {
		source := filepath.Join(filesDir, name)
		inside := filepath.Join(rootDir, target)

		if err := ensureFile(inside, writable); err != nil {
			slog.Warn("the task's root has no place for this file", "file", target, "error", err)

			continue
		}

		if err := unix.Mount(source, inside, "", unix.MS_BIND, ""); err != nil {
			return fmt.Errorf("failed to bind %s into the task's root: %w", target, err)
		}
	}

	return nil
}

// ensureFile makes sure there is a file at path to bind over, making one when
// the root can be written to. A symlink is replaced rather than followed: it
// could point anywhere, the agent's own files included.
func ensureFile(path string, writable bool) error {
	info, err := os.Lstat(path)

	switch {
	case err == nil && info.Mode().IsRegular():
		return nil
	case !writable && err == nil:
		return fmt.Errorf("%s is not a regular file", path)
	case !writable:
		return err
	case err == nil:
		if err := os.Remove(path); err != nil {
			return err
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	return file.Close()
}

// writeFiles writes the files the agent keeps for the task.
func writeFiles(config guest.Config) error {
	files := map[string][]byte{
		"hosts":       hostsFile(config.Hostname, config.Interfaces, config.Hosts),
		"resolv.conf": resolvConf(config.Nameservers),
		"hostname":    []byte(config.Hostname + "\n"),
	}

	for name, content := range files {
		if err := writeInPlace(filepath.Join(filesDir, name), content); err != nil {
			return err
		}
	}

	return nil
}

// writeInPlace rewrites a file without replacing it. A file bound elsewhere is
// bound by what it is rather than by its name, so one written anew would leave
// the binding showing the old one.
func writeInPlace(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	if _, err := file.Write(content); err != nil {
		file.Close()

		return err
	}

	return file.Close()
}

// hostsFile is the machine's /etc/hosts: itself, under its own name on each of
// its addresses, and its neighbours under theirs.
func hostsFile(hostname string, interfaces []guest.Interface, hosts []guest.Host) []byte {
	var content strings.Builder

	content.WriteString("127.0.0.1\tlocalhost\n")
	content.WriteString("::1\tlocalhost ip6-localhost ip6-loopback\n")

	for _, i := range interfaces {
		address, _, _ := strings.Cut(i.Address, "/")
		if len(address) > 0 && len(hostname) > 0 {
			fmt.Fprintf(&content, "%s\t%s\n", address, hostname)
		}
	}

	for _, host := range hosts {
		if len(host.Address) > 0 && len(host.Names) > 0 {
			fmt.Fprintf(&content, "%s\t%s\n", host.Address, strings.Join(host.Names, " "))
		}
	}

	return []byte(content.String())
}

// resolvConf is the machine's /etc/resolv.conf. A machine that cannot reach
// the internet is given no nameserver, so a lookup fails at once rather than
// waiting on one it cannot reach.
func resolvConf(nameservers []string) []byte {
	var content strings.Builder

	for _, nameserver := range nameservers {
		fmt.Fprintf(&content, "nameserver %s\n", nameserver)
	}

	return []byte(content.String())
}

// unmountRoot takes the task's root apart again, as far as it can.
func unmountRoot() {
	for _, dir := range []string{rootDir, scratchDir, imageDir} {
		_ = unix.Unmount(dir, unix.MNT_DETACH)
	}
}
