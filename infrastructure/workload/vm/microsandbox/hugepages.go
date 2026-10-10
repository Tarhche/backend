package microsandbox

import (
	"os"
	"strings"
)

// hugePagesPath is where the kernel says when it backs a process's memory
// with transparent huge pages.
const hugePagesPath = "/sys/kernel/mm/transparent_hugepage/enabled"

// hugePagesAlways is the mode in which a VM's memory is backed by huge pages.
//
// A VM's memory is an anonymous mapping of its msb process, which microsandbox
// does not madvise, so only a host whose transparent huge pages are always on
// backs it with huge pages. Any other faults it in 4 KiB at a time: the host
// takes a fault for every page the guest touches for the first time, and for
// every one it touches again after handing it back as free. That is cheap on
// bare metal, and ruinous under nested virtualization, where each of those
// faults is handled by a hypervisor that is itself a guest: in a VM in Lima,
// 52 ms for every MiB a VM touches, so that building a Go snippet takes
// minutes rather than seconds.
const hugePagesAlways = "always"

// hostHugePages is the mode the host's transparent huge pages are in, or
// nothing when the host does not say.
func hostHugePages() string {
	enabled, err := os.ReadFile(hugePagesPath)
	if err != nil {
		return ""
	}

	return hugePagesMode(string(enabled))
}

// hugePagesMode reads the mode transparent huge pages are in off what the
// kernel says of them: every mode there is, and the one they are in between
// brackets.
func hugePagesMode(enabled string) string {
	_, rest, found := strings.Cut(enabled, "[")
	if !found {
		return ""
	}

	mode, _, found := strings.Cut(rest, "]")
	if !found {
		return ""
	}

	return mode
}
