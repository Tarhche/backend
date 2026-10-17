//go:build microsandbox

package microsandbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"syscall"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// diskUsageEvery is how often a VM's disk is measured from inside it. It is a
// command run in the guest, which is cheap but not free, and a disk fills
// slowly.
const diskUsageEvery = 2 * time.Minute

// hostDiskEvery is how often a run's disk is measured from the host: what its
// sandbox's directory has allocated is read off the host's own filesystem,
// which is cheaper than a command in a guest but not free, and its node
// samples a running run every other second.
const hostDiskEvery = 10 * time.Second

// diskSample is the last measure of a VM's disk, from inside it, or from the
// host for an instance with a main process.
type diskSample struct {
	at    time.Time
	used  uint64
	total uint64
	ok    bool
}

// Stats is what a running instance is using.
//
// microsandbox counts CPU per vCPU, so a VM with two busy vCPUs is at 200%;
// the engine says how busy the VM keeps all of its vCPUs together, 0 to 100.
// A flat root disk does not say how much of it is used, so a VM's disk is
// measured from inside it every few minutes; what the host has allocated to
// it, which never shrinks, stands in until then. An instance with a main
// process, a code runner's run, has a managed disk, which microsandbox 0.7.8
// says nothing of, and nothing is exec'd into it but what it was asked to
// run: what its sandbox has allocated on the host for it stands in, measured
// from the host every few seconds, and never shrinks either.
func (e *engine) Stats(ctx context.Context, id string) (vm.Stats, error) {
	i, err := e.get(id)
	if err != nil {
		return vm.Stats{}, err
	}

	h, err := e.sandboxOf(ctx, id)
	if err != nil {
		return vm.Stats{}, err
	}

	if h == nil || h.Status() != msb.SandboxStatusRunning {
		return vm.Stats{}, fmt.Errorf("%w: %q is not running", vm.ErrNotRunning, id)
	}

	metrics, err := h.Metrics(ctx)
	if err != nil {
		return vm.Stats{}, err
	}

	r := i.current()

	stats := vm.Stats{
		CPUPercent:  min(max(metrics.CPUPercent/float64(max(r.Spec.Resources.CPUs, 1)), 0), 100),
		MemoryUsed:  metrics.MemoryBytes,
		MemoryLimit: metrics.MemoryLimitBytes,
		NetworkRx:   metrics.NetRxBytes,
		NetworkTx:   metrics.NetTxBytes,
		DiskTotal:   r.Spec.Resources.Disk,
		SampledAt:   time.Now(),
	}

	if metrics.UpperHostAllocatedBytes != nil {
		stats.DiskUsed = *metrics.UpperHostAllocatedBytes
	}

	switch {
	case metrics.UpperUsedBytes != nil && metrics.UpperFreeBytes != nil:
		// a managed disk says itself what of it is used.
		stats.DiskUsed = *metrics.UpperUsedBytes
		stats.DiskTotal = *metrics.UpperUsedBytes + *metrics.UpperFreeBytes
	case !r.Spec.HasMainProcess():
		// a main process's instance is not measured from inside: nothing is
		// exec'd into it but what it was asked to run.
		if sample := e.diskOf(ctx, i); sample.ok {
			stats.DiskUsed, stats.DiskTotal = sample.used, sample.total
		}
	case metrics.UpperHostAllocatedBytes == nil:
		// and microsandbox says nothing of its managed disk, so it is measured
		// from the host: what its sandbox has allocated there for it stands in
		// for what of it is used, as the host's allocation does for a VM's
		// disk, and never shrinks, whatever is deleted inside it.
		if sample := e.hostDiskOf(ctx, i); sample.ok {
			stats.DiskUsed = sample.used
		}
	}

	return stats, nil
}

// hostDiskOf is what the sandbox of an instance with a main process has
// allocated on the host for its disk, measured again when the last measure is
// old.
func (e *engine) hostDiskOf(ctx context.Context, i *instance) diskSample {
	i.mu.Lock()
	last := i.hostDisk
	i.mu.Unlock()

	if !last.at.IsZero() && time.Since(last.at) < hostDiskEvery {
		return last
	}

	sample := diskSample{at: time.Now()}
	sample.used, sample.ok = e.measureHostDisk(ctx, i)

	i.mu.Lock()
	i.hostDisk = sample
	i.mu.Unlock()

	return sample
}

// measureHostDisk is what an instance's sandbox has allocated on the host for
// its disk: what microsandbox observes its sandbox's own directory to have
// allocated, from the host, with nothing exec'd into the guest, less what the
// logs kept there have. Those are what its main process printed and what
// microsandbox said of it, kept beside its disk rather than on it, and grow
// with every line a run prints. A microsandbox that cannot say, as one whose
// native library is too old cannot, measures nothing, which is said at debug.
func (e *engine) measureHostDisk(ctx context.Context, i *instance) (uint64, bool) {
	sb, err := e.live(ctx, i)
	if err != nil {
		e.logger.Debug("a vm's disk could not be measured from the host", "vm", i.id, "error", err)

		return 0, false
	}

	usage, err := sb.StorageUsage(ctx)
	if err == nil && usage.AllocatedBytes == nil {
		err = errors.New("microsandbox did not say what the sandbox has allocated")
	}

	if err != nil {
		e.logger.Debug("a vm's disk could not be measured from the host", "vm", i.id, "error", err)

		return 0, false
	}

	allocated := *usage.AllocatedBytes
	logs := allocatedUnder(filepath.Join(usage.Path, "logs"))

	return allocated - min(logs, allocated), true
}

// allocatedUnder is what the files under dir have allocated on the host's
// filesystem, and nothing for a dir that is not there.
func allocatedUnder(dir string) uint64 {
	var allocated uint64

	_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}

		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Blocks > 0 {
			allocated += uint64(stat.Blocks) * 512
		}

		return nil
	})

	return allocated
}

// diskOf is a VM's disk as the guest measures it, measured again when the
// last measure is old.
func (e *engine) diskOf(ctx context.Context, i *instance) diskSample {
	i.mu.Lock()
	last := i.disk
	i.mu.Unlock()

	if !last.at.IsZero() && time.Since(last.at) < diskUsageEvery {
		return last
	}

	sample := diskSample{at: time.Now()}
	sample.used, sample.total, sample.ok = e.measureDisk(ctx, i)

	i.mu.Lock()
	i.disk = sample
	i.mu.Unlock()

	return sample
}

// measureDisk asks the guest how big its root filesystem is and how much of
// it is used. What df says is left in a file and read back through the agent,
// since an exec's output is kept as the VM's log.
func (e *engine) measureDisk(ctx context.Context, i *instance) (used uint64, total uint64, ok bool) {
	sb, err := e.live(ctx, i)
	if err != nil {
		return 0, 0, false
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	out, err := sb.Shell(ctx, diskUsageScript)
	if err != nil || out.ExitCode() != 0 {
		return 0, 0, false
	}

	report, err := sb.FS().ReadString(ctx, diskUsageFile)
	if err != nil {
		return 0, 0, false
	}

	return parseDF(report)
}

// Logs is what an instance's log holds: what the guest's console and
// microsandbox's runtime said, its main process's output, and every exec's.
//
// microsandbox keeps a sandbox's log whether it runs or not, and reads all of
// it back at once; since is inclusive, and a line written at that very moment
// is kept. A main process that never started says why, under its own source.
func (e *engine) Logs(ctx context.Context, id string, options vm.LogOptions) ([]vm.LogLine, error) {
	i, err := e.get(id)
	if err != nil {
		return nil, err
	}

	r := i.current()

	var lines []vm.LogLine

	h, err := e.sandboxOf(ctx, id)
	if err != nil {
		return nil, err
	}

	if h != nil {
		select {
		case e.logReads <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		lines, err = e.readLogs(ctx, h, r.Spec.HasMainProcess(), options)
		<-e.logReads

		if err != nil {
			return nil, err
		}
	}

	return window(merged(lines, engineLines(r)), options), nil
}

// readLogs reads what microsandbox kept of a sandbox's log, as lines.
//
// microsandbox's tail counts entries, and an entry holds one line or more,
// so the last entries asked for hold at least as many lines as were asked
// for; when some of them are left out for being older than since, more are
// asked for.
func (e *engine) readLogs(ctx context.Context, h *msb.SandboxHandle, hasMain bool, options vm.LogOptions) ([]vm.LogLine, error) {
	request := msb.LogOptions{
		Sources: []msb.LogSource{msb.LogSourceStdout, msb.LogSourceStderr, msb.LogSourceOutput, msb.LogSourceSystem},
		Since:   options.Since,
		Tail:    uint64(options.Tail),
	}

	for {
		entries, err := h.Logs(ctx, request)
		if err != nil {
			return nil, err
		}

		read := make([]logEntry, len(entries))
		for n, entry := range entries {
			read[n] = logEntry{source: string(entry.Source), session: entry.SessionID, at: entry.Timestamp, data: entry.Data}
		}

		lines := window(linesOf(read, hasMain), vm.LogOptions{Since: options.Since})

		if options.Tail == 0 || uint(len(lines)) >= options.Tail || uint64(len(entries)) < request.Tail {
			return lines, nil
		}

		request.Tail *= 4
	}
}

// engineLines are what the engine itself says in an instance's log: why its
// main process did not run, which the process never got to say.
func engineLines(r *record) []vm.LogLine {
	if r.Exit == nil || len(r.Exit.Reason) == 0 {
		return nil
	}

	return []vm.LogLine{{At: r.Exit.At, Source: vm.LogSourceMain, Line: r.Exit.Reason}}
}
