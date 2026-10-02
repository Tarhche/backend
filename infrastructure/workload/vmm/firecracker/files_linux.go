//go:build linux

package firecracker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// install copies the firecracker binary machines run into dir, named by what
// it holds, unless it is there already, and says where it is. Machines'
// firecrackers are linked from there, so the one a machine was started from
// is never replaced under it, however often vmhost is upgraded; and a binary
// that is installed already, by whatever installed it, is taken as it is.
func install(source string, dir string) (string, error) {
	binary, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("%s is not there to start machines with: %w", source, err)
	}
	defer binary.Close()

	info, err := binary.Stat()
	if err != nil {
		return "", err
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a file to start machines with", source)
	}

	digest := sha256.New()
	if _, err := io.Copy(digest, binary); err != nil {
		return "", err
	}

	target := filepath.Join(dir, execName+"-"+hex.EncodeToString(digest.Sum(nil))[:16])

	// every machine's user runs it, through the link beside its directory.
	const mode = 0o755

	if installed, err := os.Lstat(target); err == nil {
		if !installed.Mode().IsRegular() {
			return "", fmt.Errorf("%s is in the way of the firecracker machines run", target)
		}

		return target, os.Chmod(target, mode)
	}

	if _, err := binary.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	// copied beside where it goes and moved there whole, so nothing ever
	// starts half of one.
	temporary, err := os.CreateTemp(dir, ".firecracker-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary.Name())

	if _, err := io.Copy(temporary, binary); err != nil {
		temporary.Close()

		return "", err
	}

	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()

		return "", err
	}

	if err := temporary.Close(); err != nil {
		return "", err
	}

	if err := os.Rename(temporary.Name(), target); err != nil {
		return "", err
	}

	return target, nil
}

// prepare makes a machine's directory and links into it what the machine
// boots from, and only then gives the machine's own part of it to the
// machine's user: nothing runs as that user while it is made.
//
// The machine's directory is vmhost's, and its user goes through it without
// seeing anything; the firecracker it runs is linked there, where its user
// cannot replace it. In it, the directory the machine runs in is its user's
// alone, as is the directory its firecracker makes its sockets in.
func (h *Hypervisor) prepare(spec vm.MachineSpec) error {
	machine, root := h.machineDir(spec.ID), h.rootDir(spec.ID)
	run := filepath.Join(root, filepath.Dir(layout.APISocketName))

	for _, dir := range []struct {
		path string
		mode os.FileMode
	}{
		{path: machine, mode: 0o711},
		{path: root, mode: 0o700},
		{path: run, mode: 0o700},
	} {
		if err := os.Mkdir(dir.path, dir.mode); err != nil {
			return err
		}

		// whatever the umask left of the mode, it is the one asked for.
		if err := os.Chmod(dir.path, dir.mode); err != nil {
			return err
		}
	}

	if err := h.link(h.binary, filepath.Join(machine, h.execName), spec.UID, shared); err != nil {
		return err
	}

	if err := h.link(spec.Kernel, filepath.Join(root, kernelName), spec.UID, shared); err != nil {
		return err
	}

	if len(spec.Initrd) > 0 {
		if err := h.link(spec.Initrd, filepath.Join(root, initrdName), spec.UID, shared); err != nil {
			return err
		}
	}

	for i, drive := range spec.Drives {
		access := shared
		if !drive.ReadOnly {
			access = own
		}

		if err := h.link(drive.Path, filepath.Join(root, driveName(i)), spec.UID, access); err != nil {
			return err
		}
	}

	if spec.UID == 0 {
		return nil
	}

	for _, dir := range []string{run, root} {
		if err := os.Lchown(dir, spec.UID, spec.UID); err != nil {
			return err
		}
	}

	return nil
}

// access is what a machine does with a file it boots from.
type access int

const (
	// shared is a file a machine only reads, which every machine may share:
	// it has to be everybody's to read already, and is left as it is.
	shared access = iota

	// own is a file the machine writes, which is made its user's alone.
	own
)

// link links a file a machine boots from to target, in the machine's
// directory.
//
// A file has to be under the data directory, has to be a file rather than
// anything pointing elsewhere, and is linked by what it is rather than by its
// name, so nothing swapped in on the way can take it anywhere else, before this
// or while the machine runs. Being linked, it needs no way through the
// directories it is kept in, which are vmhost's alone.
func (h *Hypervisor) link(source string, target string, uid int, access access) error {
	if !filepath.IsAbs(source) {
		return fmt.Errorf("%w: %q is not an absolute path", vm.ErrInvalid, source)
	}

	relative, err := filepath.Rel(h.config.DataDir, filepath.Clean(source))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return fmt.Errorf("%w: %s is not under %s, where machines boot from", vm.ErrInvalid, source, h.config.DataDir)
	}

	dataDir, err := os.Open(h.config.DataDir)
	if err != nil {
		return err
	}
	defer dataDir.Close()

	// the file is found from the data directory down, through nothing that is
	// a symlink and never above where it started: a directory swapped for a
	// symlink cannot take it anywhere else.
	fd, err := unix.Openat2(int(dataDir.Fd()), relative, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return fmt.Errorf("%w: %s is not a file a machine can boot from: %v", vm.ErrInvalid, source, err)
	}
	defer unix.Close(fd)

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}

	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("%w: %s is not a file", vm.ErrInvalid, source)
	}

	if uid > 0 && access == shared && stat.Mode&0o004 == 0 {
		return fmt.Errorf("%w: %s is not everybody's to read, which a file machines share has to be", vm.ErrInvalid, source)
	}

	// the handle this process holds on the file stands for the file by what
	// it is. Linking the handle itself takes a privilege vmhost need not
	// hold; linking through its name under /proc does not.
	handle := "/proc/self/fd/" + strconv.Itoa(fd)

	if err := unix.Linkat(unix.AT_FDCWD, handle, unix.AT_FDCWD, target, unix.AT_SYMLINK_FOLLOW); err != nil {
		if errors.Is(err, unix.EXDEV) {
			return fmt.Errorf("%w: %s has to be on the same filesystem as %s: %v", vm.ErrUnavailable, source, h.config.DataDir, err)
		}

		return fmt.Errorf("%s cannot be linked into the machine's directory: %w", source, err)
	}

	if access == shared || uid == 0 {
		return nil
	}

	if err := unix.Fchownat(unix.AT_FDCWD, handle, uid, uid, 0); err != nil {
		return err
	}

	return unix.Fchmodat(unix.AT_FDCWD, handle, 0o600, 0)
}

// deviceGroups are the groups a machine's process has to be in to open the
// devices it uses: none for a device open to everybody, and the device's own
// for one open to its group.
func deviceGroups(devices ...string) ([]uint32, error) {
	var groups []uint32

	for _, device := range devices {
		var stat unix.Stat_t
		if err := unix.Stat(device, &stat); err != nil {
			return nil, fmt.Errorf("%w: %s is not there for machines to use: %v", vm.ErrUnavailable, device, err)
		}

		switch {
		case stat.Mode&0o006 == 0o006:
		case stat.Mode&0o060 == 0o060:
			if !slices.Contains(groups, stat.Gid) {
				groups = append(groups, stat.Gid)
			}
		default:
			return nil, fmt.Errorf("%w: %s is open to its owner alone, and no machine runs as its owner", vm.ErrUnavailable, device)
		}
	}

	return groups, nil
}

// ownerOf is who owns a file.
func ownerOf(path string) (int, bool) {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return 0, false
	}

	return int(stat.Uid), true
}
