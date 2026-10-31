package microsandbox

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// managedJournal is what a managed root disk's ext4 keeps of itself
	// besides about 4% of the rest: a fixed 64 MiB journal, and a margin.
	// What is left to write is about 0.96·N − 65 MiB of a disk of N.
	managedJournal = 72 * mib

	// managedFloor is the smallest managed root disk microsandbox formats and
	// boots.
	managedFloor = 70

	// managedRestoreFloor is the least disk, in bytes, a VM restored from a
	// snapshot of a managed root disk is given, so that its disk is the one
	// it is given: 5 GiB. microsandbox, as of 0.7.11, records a managed disk
	// it restores as 4 GiB, its default, whatever the disk was, and grows it
	// only to more than that: less is refused as a shrink, and 4 GiB itself
	// as no change. Its guest then grows ext4 into a last 128 MiB block group
	// only when that group has room for its own metadata, a 2 MiB inode table
	// and two bitmaps, which 4 GiB and 1 or 2 MiB have not, so 4 GiB and
	// 3 MiB is the least it grows to. The floor is the next whole GiB, a size
	// the dashboard shows as it is and a person can type, and a whole number
	// of block groups. A VM restored with less than 4 GiB and 3 MiB keeps the
	// disk the snapshot was taken of, the hundred MiB or so of a code
	// runner's run, whatever its record says.
	managedRestoreFloor = 5 << 30
)

// vcpus is the vCPUs microsandbox is asked for: one at the least, and no more
// than it can be given.
func vcpus(cpus uint) (uint8, error) {
	if cpus > math.MaxUint8 {
		return 0, fmt.Errorf("%d vCPUs are more than a VM can be given", cpus)
	}

	return uint8(max(cpus, 1)), nil
}

// mebibytes is bytes as the whole MiB microsandbox takes them in, rounded up
// so that nothing is given less than it asked for. Zero leaves it to
// microsandbox's default.
func mebibytes(bytes uint64) (uint32, error) {
	n := bytes / mib
	if bytes%mib != 0 {
		n++
	}

	if n > math.MaxUint32 {
		return 0, fmt.Errorf("%d bytes are more than a VM can be given", bytes)
	}

	return uint32(n), nil
}

// managedDiskMiB is the managed root disk, in MiB, that leaves a workload at
// least limit bytes to write: its filesystem keeps a journal and a share of
// the disk to itself, so the disk is made that much larger.
func managedDiskMiB(limit uint64) (uint32, error) {
	if limit > math.MaxUint64/100-managedJournal {
		return 0, fmt.Errorf("%d bytes are more than a VM can be given", limit)
	}

	// ceil((limit + journal) / 0.96), in bytes, then in whole MiB.
	bytes := ((limit+managedJournal)*100 + 95) / 96

	size, err := mebibytes(bytes)
	if err != nil {
		return 0, err
	}

	return max(size, managedFloor), nil
}

// parseDF reads the size and the use of a filesystem, in bytes, off what
// POSIX df says of it in bytes: a header line, then the filesystem's.
func parseDF(report string) (used uint64, total uint64, ok bool) {
	lines := strings.Split(strings.TrimSpace(report), "\n")
	if len(lines) < 2 {
		return 0, 0, false
	}

	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0, 0, false
	}

	total, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	used, err = strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	return used, total, true
}
