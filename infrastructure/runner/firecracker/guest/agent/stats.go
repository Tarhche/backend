package agent

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// cpuSample is how much CPU the task had used, and when that was read.
type cpuSample struct {
	usage uint64
	at    time.Time
}

// cpuPercent is the share of one CPU a task used between two samples.
func cpuPercent(previous cpuSample, current cpuSample) float64 {
	elapsed := current.at.Sub(previous.at)
	if previous.at.IsZero() || elapsed <= 0 || current.usage < previous.usage {
		return 0
	}

	used := time.Duration(current.usage-previous.usage) * time.Microsecond

	return float64(used) / float64(elapsed) * 100
}

// memoryTotal is how much memory the machine has, from /proc/meminfo.
func memoryTotal(meminfo string) uint64 {
	scanner := bufio.NewScanner(strings.NewReader(meminfo))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kilobytes, _ := strconv.ParseUint(fields[1], 10, 64)

			return kilobytes * 1024
		}
	}

	return 0
}

// networkTotals is what went in and out of the machine's network devices,
// from /proc/net/dev. The loopback is the machine talking to itself.
func networkTotals(netdev string) (uint64, uint64) {
	var received, sent uint64

	scanner := bufio.NewScanner(strings.NewReader(netdev))
	for scanner.Scan() {
		name, counters, found := strings.Cut(scanner.Text(), ":")
		if !found || strings.TrimSpace(name) == "lo" {
			continue
		}

		fields := strings.Fields(counters)
		if len(fields) < 9 {
			continue
		}

		in, _ := strconv.ParseUint(fields[0], 10, 64)
		out, _ := strconv.ParseUint(fields[8], 10, 64)

		received += in
		sent += out
	}

	return received, sent
}

// diskTotals is what was read from and written to the machine's disks, from
// /proc/diskstats. Sectors are 512 bytes whatever the disk says.
func diskTotals(diskstats string) (uint64, uint64) {
	var read, written uint64

	scanner := bufio.NewScanner(strings.NewReader(diskstats))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 || !isWholeDisk(fields[2]) {
			continue
		}

		sectorsRead, _ := strconv.ParseUint(fields[5], 10, 64)
		sectorsWritten, _ := strconv.ParseUint(fields[9], 10, 64)

		read += sectorsRead * 512
		written += sectorsWritten * 512
	}

	return read, written
}

// isWholeDisk reports whether a device is one of the machine's virtio disks
// rather than a partition of one.
func isWholeDisk(name string) bool {
	if !strings.HasPrefix(name, "vd") || len(name) < 3 {
		return false
	}

	for _, r := range name[2:] {
		if r < 'a' || r > 'z' {
			return false
		}
	}

	return true
}

func readText(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(content)
}
