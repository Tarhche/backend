//go:build microsandbox

package microsandbox

import (
	"context"
	"fmt"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// diskUsageEvery is how often a VM's disk is measured from inside it. It is a
// command run in the guest, which is cheap but not free, and a disk fills
// slowly.
const diskUsageEvery = 2 * time.Minute

// diskSample is the last measure of a VM's disk from inside it.
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
// it, which never shrinks, stands in until then.
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
	}

	return stats, nil
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
