//go:build linux

package agent

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// followLogs streams the task's output after the line it was asked about, one
// JSON line each, and with follow keeps streaming what comes.
func (a *Agent) followLogs(rw http.ResponseWriter, r *http.Request) {
	var after uint64

	if value := r.URL.Query().Get(guest.QueryAfter); len(value) > 0 {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			http.Error(rw, "that is not a line number", http.StatusBadRequest)

			return
		}

		after = parsed
	}

	follow := r.URL.Query().Get(guest.QueryFollow) == "1"

	rw.Header().Set("Content-Type", "application/x-ndjson")

	encoder := json.NewEncoder(rw)
	flusher, _ := rw.(http.Flusher)

	for {
		lines, changed := a.logs.after(after)

		for _, line := range lines {
			if err := encoder.Encode(line); err != nil {
				return
			}

			after = line.Seq
		}

		if flusher != nil {
			flusher.Flush()
		}

		if !follow {
			return
		}

		select {
		case <-changed:
		case <-r.Context().Done():
			return
		}
	}
}

// stats says what the task is using. CPU is counted since the last time it
// was asked, the way a container's is.
func (a *Agent) stats(rw http.ResponseWriter, r *http.Request) {
	used := a.confine.usage()

	current := cpuSample{usage: used.cpu, at: time.Now()}

	a.lock.Lock()
	previous := a.cpu
	a.cpu = current
	a.lock.Unlock()

	total, available := memoryInfo(readText("/proc/meminfo"))
	received, sent := networkTotals(readText("/proc/net/dev"))
	read, written := diskTotals(readText("/proc/diskstats"))

	answer(rw, guest.Stats{
		PIDs:          used.pids,
		CPUPercent:    cpuPercent(previous, current),
		MemoryUsage:   total - available,
		MemoryLimit:   total,
		NetworkInput:  received,
		NetworkOutput: sent,
		BlockInput:    read,
		BlockOutput:   written,
	})
}
