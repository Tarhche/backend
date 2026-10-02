package image

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// formatter makes an empty filesystem on the disk at path.
type formatter func(ctx context.Context, path string) error

// formatWith formats disks as ext4 with the mke2fs at binary.
//
// No blocks are kept back for root (-m 0): the disk's size is the most the
// task can write, whoever it runs as. The disk is a sparse file that has never
// been written, so everything in it already reads as zeros: mke2fs is let off
// zeroing the journal (lazy_journal_init), as it lets itself off zeroing the
// inode tables of a disk it has discarded. Zeroed, the journal alone would
// take 32 MiB of a 1 GiB disk on the host, and 64 MiB of a 10 GiB one, before
// the task wrote anything; this way such a disk takes under 5 MiB.
func formatWith(binary string) formatter {
	return func(ctx context.Context, path string) error {
		output, err := exec.CommandContext(ctx, binary, "-q", "-F", "-t", "ext4", "-m", "0", "-E", "lazy_journal_init=1", "-L", "scratch", path).CombinedOutput()
		if err != nil {
			return fmt.Errorf("mke2fs failed: %w: %s", err, strings.TrimSpace(string(output)))
		}

		return nil
	}
}

// MakeScratch makes the disk a writable task keeps its changes on, at path:
// empty, sparse, formatted, and size bytes large, which is the most the task
// can write. It is made whole beside path and moved there once it is, so a
// disk is either all there or not there at all.
//
// The disk is vmhost's own, and nobody else's to read, until the hypervisor
// gives it to the user the machine runs as.
func (s *Store) MakeScratch(ctx context.Context, path string, size uint64) error {
	if size == 0 || size > math.MaxInt64 {
		return fmt.Errorf("%w: a scratch disk of %d bytes cannot be made", vm.ErrInvalid, size)
	}

	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: %q is not a path a scratch disk can be made at", vm.ErrInvalid, path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	// made for vmhost alone (0600), as a temporary file is.
	disk, err := os.CreateTemp(filepath.Dir(path), ".scratch-")
	if err != nil {
		return err
	}
	defer os.Remove(disk.Name())

	if err := disk.Truncate(int64(size)); err != nil {
		return errors.Join(err, disk.Close())
	}

	if err := disk.Close(); err != nil {
		return err
	}

	if err := s.format(ctx, disk.Name()); err != nil {
		return fmt.Errorf("failed to make a scratch disk: %w", err)
	}

	return os.Rename(disk.Name(), path)
}
