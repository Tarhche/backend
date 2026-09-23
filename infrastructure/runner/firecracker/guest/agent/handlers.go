//go:build linux

package agent

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

// followLogs streams the task's output after the line it was asked about, one
// JSON line each, and with follow keeps streaming what comes.
func (a *Agent) followLogs(rw http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	follow := r.URL.Query().Get("follow") == "1"

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
	current := cpuSample{
		usage: readKeyed(taskCgroup+"/cpu.stat", "usage_usec"),
		at:    time.Now(),
	}

	a.lock.Lock()
	previous := a.cpu
	a.cpu = current
	a.lock.Unlock()

	received, sent := networkTotals(readText("/proc/net/dev"))
	read, written := diskTotals(readText("/proc/diskstats"))

	answer(rw, guest.Stats{
		PIDs:          readCounter(taskCgroup + "/pids.current"),
		CPUPercent:    cpuPercent(previous, current),
		MemoryUsage:   readCounter(taskCgroup + "/memory.current"),
		MemoryLimit:   memoryTotal(readText("/proc/meminfo")),
		NetworkInput:  received,
		NetworkOutput: sent,
		BlockInput:    read,
		BlockOutput:   written,
	})
}
