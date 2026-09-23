// Package layout is how the state directory the launcher and the orchestrators
// share is laid out, which both of them have to agree on.
//
//	boot/vmlinux                       the kernel machines boot, installed by the launcher
//	boot/initrd-<digest>.cpio.gz       the initramfs holding the agent, built by an orchestrator
//	images/<digest>/rootfs.ext4        an image's root, shared by every machine that runs it
//	nodes/<orchestrator>/machines/<id> a machine's record, disks and output
//	j/…                                machines' own directories, the launcher's alone
//	launcher.sock                      where the launcher takes orders
//
// The state directory itself, j/ and the socket are the launcher's, and root's.
// Everything an orchestrator writes is under boot/, images/ and nodes/, which
// are made for whoever machines run as. So an orchestrator can make what its
// machines boot from, and cannot touch where the launcher keeps them.
package layout

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	BootDir   = "boot"
	ImagesDir = "images"
	NodesDir  = "nodes"

	// JailDir is where machines' own directories are made. It is short on
	// purpose: a socket's path is at most 108 bytes, and a machine's lives
	// several directories down from it.
	JailDir = "j"

	// SocketName is what the launcher's socket is called in the state
	// directory.
	SocketName = "launcher.sock"

	// KernelName is what the installed kernel is called in BootDir.
	KernelName = "vmlinux"

	machinesDir = "machines"
)

// Kernel is where the installed kernel is.
func Kernel(stateDir string) string {
	return filepath.Join(stateDir, BootDir, KernelName)
}

// Boot is where kernels and initramfs are.
func Boot(stateDir string) string {
	return filepath.Join(stateDir, BootDir)
}

// Images is where images' roots are.
func Images(stateDir string) string {
	return filepath.Join(stateDir, ImagesDir)
}

// Machines is where one orchestrator keeps its machines.
func Machines(stateDir string, owner string) string {
	return filepath.Join(stateDir, NodesDir, owner, machinesDir)
}

// Prepare lays the state directory out for machines to be made in, and
// installs the kernel they boot.
func Prepare(stateDir string, kernel string, uid int, gid int) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}

	info, err := os.Lstat(stateDir)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", stateDir)
	}

	for _, dir := range []string{BootDir, ImagesDir, NodesDir} {
		path := filepath.Join(stateDir, dir)

		if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}

		if err := os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("failed to give %s to whoever machines run as: %w", path, err)
		}
	}

	if len(kernel) == 0 {
		return nil
	}

	return install(kernel, Kernel(stateDir))
}

// install copies a file into place whole, so nothing ever reads half of one.
func install(source string, target string) error {
	from, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("the kernel machines boot is not at %s: %w", source, err)
	}
	defer from.Close()

	temporary, err := os.CreateTemp(filepath.Dir(target), ".install-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())

	if _, err := io.Copy(temporary, from); err != nil {
		temporary.Close()

		return err
	}

	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()

		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporary.Name(), target)
}
