package profiler

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// newMemoryCollector captures heap profiles and records GC pause durations on
// every tick. The heap profile carries all four memory sample types
// (alloc_objects, alloc_space, inuse_objects, inuse_space), so a separate
// "allocs" collection would duplicate the exact same data and is skipped.
func newMemoryCollector(
	cfg Config,
	monitor *cpuMonitor,
	guard *resourceGuard,
	inst *instruments,
	sink func(context.Context, ProfileType, []byte),
	logger *slog.Logger,
) *lookupCollector {
	recorder := newGCPauseRecorder(inst)

	return newLookupCollector("heap", ProfileTypeHeap, cfg.MemoryInterval, cfg, monitor, guard, inst, sink, logger, recorder.record)
}

// gcPauseRecorder feeds the pauses of completed GC cycles into the pause
// histogram.
type gcPauseRecorder struct {
	inst      *instruments
	lastNumGC uint32
}

func newGCPauseRecorder(inst *instruments) *gcPauseRecorder {
	// baseline so pauses from before the profiler started are not recorded
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	return &gcPauseRecorder{inst: inst, lastNumGC: ms.NumGC}
}

// record emits the pauses of GC cycles completed since the previous call.
// PauseNs is a circular buffer of the most recent 256 pauses; older ones are
// lost, which is acceptable at this cadence.
func (g *gcPauseRecorder) record(ctx context.Context) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	last := g.lastNumGC
	if ms.NumGC > last+uint32(len(ms.PauseNs)) {
		last = ms.NumGC - uint32(len(ms.PauseNs))
	}

	// the pause of GC cycle k lives at PauseNs[(k+255)%256]
	for k := last + 1; k <= ms.NumGC; k++ {
		pause := ms.PauseNs[(k+uint32(len(ms.PauseNs))-1)%uint32(len(ms.PauseNs))]
		g.inst.recordGCPause(ctx, time.Duration(pause))
	}

	g.lastNumGC = ms.NumGC
}
