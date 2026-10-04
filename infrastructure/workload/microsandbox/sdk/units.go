package sdk

import (
	"fmt"
	"math"
	"strings"
)

const mib = 1 << 20

// A sandbox's writable root is an ext4 image with a journal of 64 MiB, which
// microsandbox gives no way to size. Of a root of N MiB, about 0.96·N − 65 MiB
// is left for files, and microsandbox cannot make one of under 69 MiB at all,
// as measured on microsandbox 0.7.6. So a disk limit asks for a root of
// (limit + diskOverheadMiB) / 0.96, which holds at least the limit, and never
// for less than minRootDiskMiB.
const (
	diskOverheadMiB = 72
	minRootDiskMiB  = 70
)

// vcpus is the whole vCPUs a sandbox boots with for a limit in cores: rounded
// up, so that a run never gets less than it asked for, and at least one. That
// is also why a fractional limit is not enforced.
func vcpus(cores float64) (uint8, error) {
	if math.IsNaN(cores) || math.IsInf(cores, 0) {
		return 0, fmt.Errorf("microsandbox: %v is not a number of CPUs", cores)
	}

	if cores <= 1 {
		return 1, nil
	}

	n := math.Ceil(cores)
	if n > math.MaxUint8 {
		return 0, fmt.Errorf("microsandbox: %v CPUs is more than the %d vCPUs a sandbox can have", cores, math.MaxUint8)
	}

	return uint8(n), nil
}

// memoryMiB is the whole MiB of memory a sandbox of cpus vCPUs boots with for
// a limit of bytes: rounded up, so that a run never gets less than it asked
// for, and past the sizes guests do not boot in.
func memoryMiB(bytes uint64, cpus uint8) (uint32, error) {
	n := bootableMiB(ceilMiB(bytes), cpus)
	if n > math.MaxUint32 {
		return 0, fmt.Errorf("microsandbox: %d bytes of memory is more than a sandbox can have", bytes)
	}

	return uint32(n), nil
}

// Guests of some sizes do not boot: from 92 MiB, the guest's kernel runs out
// of memory early in its boot and panics, up to a size that grows with the
// vCPUs, about 108 MiB with one, 111 with four, 117 with eight and 127 with
// sixteen. Below 92 MiB and above that, they boot. So a size in between is
// raised to 112 MiB and 2 more for each vCPU, which clears it with room to
// spare. This was measured on arm64 with microsandbox 0.7.6 and libkrunfw
// 5.6.1, whose guest kernel is 6.12.109; a test boots every size around it, so
// a runtime that moves it shows.
const unbootableFromMiB = 92

func bootableMiB(mib uint64, cpus uint8) uint64 {
	if bootable := 112 + 2*uint64(cpus); mib >= unbootableFromMiB && mib < bootable {
		return bootable
	}

	return mib
}

// rootDiskMiB is the size of the managed root that holds a disk limit of limit
// bytes. See diskOverheadMiB for why it is larger than the limit.
func rootDiskMiB(limit uint64) (uint32, error) {
	// ceil((limit + overhead) / 0.96), in integers: 0.96 is 24/25.
	n := max(((ceilMiB(limit)+diskOverheadMiB)*25+23)/24, minRootDiskMiB)
	if n > math.MaxUint32 {
		return 0, fmt.Errorf("microsandbox: a disk of %d bytes is more than a sandbox can have", limit)
	}

	return uint32(n), nil
}

func ceilMiB(bytes uint64) uint64 {
	n := bytes / mib
	if bytes%mib != 0 {
		n++
	}

	return n
}

// envMap is a command's KEY=VALUE environment as the map the SDK takes.
func envMap(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	env := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("microsandbox: environment entry %q is not KEY=VALUE", entry)
		}

		env[key] = value
	}

	return env, nil
}

// linuxErrnos names the errors a command most often fails to start with, by
// the numbers Linux gives them, since the guest is Linux whatever the host is.
var linuxErrnos = map[int]string{
	1:  "EPERM",
	2:  "ENOENT",
	7:  "E2BIG",
	8:  "ENOEXEC",
	12: "ENOMEM",
	13: "EACCES",
	20: "ENOTDIR",
	21: "EISDIR",
	26: "ETXTBSY",
	36: "ENAMETOOLONG",
	40: "ELOOP",
}

// failureKinds are the kinds the guest agent sorts the failures to start into,
// by the error each one stands for.
var failureKinds = map[string]string{
	"not_found":         "ENOENT",
	"permission_denied": "EACCES",
	"not_executable":    "ENOEXEC",
}

// errnoName names the error that kept a command from starting. The guest
// agent reports its name, its number and a kind: the name is what the port
// carries, and the number or the kind stand in when it is missing.
func errnoName(name string, number *int, kind string) string {
	if name != "" {
		return name
	}

	if number != nil {
		if name, ok := linuxErrnos[*number]; ok {
			return name
		}

		return fmt.Sprintf("errno %d", *number)
	}

	if name, ok := failureKinds[kind]; ok {
		return name
	}

	return kind
}

// running is whether a sandbox in this state has a VM up: booting, running,
// draining or paused. A created, stopped or crashed one has none.
func running(status string) bool {
	switch status {
	case "starting", "running", "draining", "paused":
		return true
	default:
		return false
	}
}

// runtimeVersion is the version in what msb --version prints, which is
// "msb 0.7.6".
func runtimeVersion(output string) string {
	output = strings.TrimSpace(output)
	if fields := strings.Fields(output); len(fields) == 2 && fields[0] == "msb" {
		return fields[1]
	}

	return output
}

// lastLine is the last line of a command's output that says anything, which
// is where msb puts its error.
func lastLine(output []byte) string {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	return strings.TrimSpace(lines[len(lines)-1])
}
