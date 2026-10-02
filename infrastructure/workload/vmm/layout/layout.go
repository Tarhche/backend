// Package layout is how vmhost's data directory is laid out, which every part
// of vmhost has to agree on: what boots a machine, where its own directory is,
// where its record, disks and output are kept.
//
//	bin/firecracker-<digest>          the VMM, copied out of vmhost's image
//	boot/vmlinux-<digest>             the kernel machines boot, copied the same way
//	boot/initrd-<digest>.cpio.gz      the initramfs holding the agent, made from the guest binary
//	images/<digest>/rootfs.squashfs   an image's root, read only, shared by every VM that runs it
//	images/<digest>/config.json       what the image says about how it is run
//	images/refs/<hash of reference>   the digest a reference was last made under
//	vms/<id>/state.json               a VM's record
//	vms/<id>/scratch.ext4             the disk a writable VM keeps its changes on
//	vms/<id>/output.log               what its task wrote, numbered across every boot
//	fabric/                           what the fabric keeps: which address is whose
//	j/<id>/root/                      a machine's own directory, where its VMM runs as its user
//	j/<id>/console.log                what its VMM and kernel said, for operators
//
// The directory is the same path inside vmhost's container and on the host,
// because a machine's VMM runs on the host as a systemd unit, and is given
// these paths. Everything a machine boots from is linked into its own
// directory rather than copied, so it all has to be on one filesystem.
//
// Binaries, kernels and initramfs are named by what they hold, so a vmhost
// that is upgraded never overwrites one a running machine was started from.
//
// Paths are short on purpose: a unix socket's path is at most 108 bytes, and
// a machine's sockets live several directories down. Adapted from PR #101's
// layout, which the launcher and the orchestrators shared.
package layout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	BinDir     = "bin"
	BootDir    = "boot"
	ImagesDir  = "images"
	VMsDir     = "vms"
	FabricDir  = "fabric"
	MachineDir = "j"

	// the files kept for a VM.
	StateName   = "state.json"
	ScratchName = "scratch.ext4"
	OutputName  = "output.log"
	ConsoleName = "console.log"

	// RootName is a machine's own directory, inside its directory: where
	// its VMM runs, and where what it boots from is linked.
	RootName = "root"

	// the sockets a machine's VMM makes in its own directory: its API, and
	// its vsock, which the guest's agent is reached through. They are in
	// run/, the one directory there the machine's user may make anything in.
	APISocketName   = "run/firecracker.socket"
	VsockSocketName = "run/v.sock"
)

// Bin is where the VMM binaries machines run are kept.
func Bin(dataDir string) string {
	return filepath.Join(dataDir, BinDir)
}

// Boot is where kernels and initramfs are kept.
func Boot(dataDir string) string {
	return filepath.Join(dataDir, BootDir)
}

// Images is where images' roots are kept.
func Images(dataDir string) string {
	return filepath.Join(dataDir, ImagesDir)
}

// VMs is where every VM's record, disks and output are kept.
func VMs(dataDir string) string {
	return filepath.Join(dataDir, VMsDir)
}

// VM is where one VM's record, disks and output are kept.
func VM(dataDir string, id string) string {
	return filepath.Join(VMs(dataDir), id)
}

// State is a VM's record.
func State(dataDir string, id string) string {
	return filepath.Join(VM(dataDir, id), StateName)
}

// Scratch is a VM's scratch disk.
func Scratch(dataDir string, id string) string {
	return filepath.Join(VM(dataDir, id), ScratchName)
}

// Output is what a VM's task wrote.
func Output(dataDir string, id string) string {
	return filepath.Join(VM(dataDir, id), OutputName)
}

// Fabric is where the fabric keeps what it has to remember.
func Fabric(dataDir string) string {
	return filepath.Join(dataDir, FabricDir)
}

// Machines is where machines' own directories are made.
func Machines(dataDir string) string {
	return filepath.Join(dataDir, MachineDir)
}

// Machine is one machine's directory, which is vmhost's: its user goes
// through it to its own root without seeing anything on the way.
func Machine(dataDir string, id string) string {
	return filepath.Join(Machines(dataDir), id)
}

// MachineRoot is the directory a machine's VMM runs in, which is its user's.
func MachineRoot(dataDir string, id string) string {
	return filepath.Join(Machine(dataDir, id), RootName)
}

// Console is what a machine's VMM and kernel said.
func Console(dataDir string, id string) string {
	return filepath.Join(Machine(dataDir, id), ConsoleName)
}

// APISocket is where a machine's VMM takes its API.
func APISocket(dataDir string, id string) string {
	return filepath.Join(MachineRoot(dataDir, id), APISocketName)
}

// VsockSocket is where a machine's guest agent is reached.
func VsockSocket(dataDir string, id string) string {
	return filepath.Join(MachineRoot(dataDir, id), VsockSocketName)
}

// Prepare lays the data directory out for machines to be made in.
//
// The data directory and the machines' directory are gone through by
// everybody and listed by nobody: a machine's user passes through them on the
// way to its own root and sees nothing on the way. Everything else is
// vmhost's alone; what a machine boots from reaches it through what is linked
// into its own root, never through these paths.
func Prepare(dataDir string) error {
	if err := makeDir(dataDir, 0o711); err != nil {
		return err
	}

	for _, dir := range []struct {
		name string
		mode os.FileMode
	}{
		{name: BinDir, mode: 0o700},
		{name: BootDir, mode: 0o700},
		{name: ImagesDir, mode: 0o700},
		{name: VMsDir, mode: 0o700},
		{name: FabricDir, mode: 0o700},
		{name: MachineDir, mode: 0o711},
	} {
		if err := makeDir(filepath.Join(dataDir, dir.name), dir.mode); err != nil {
			return err
		}
	}

	return nil
}

// makeDir makes a directory with exactly the mode given, whatever the umask
// says, and refuses something that is there and is not a directory.
func makeDir(path string, mode os.FileMode) error {
	if err := os.Mkdir(path, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	return os.Chmod(path, mode)
}
