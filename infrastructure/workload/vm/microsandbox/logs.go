package microsandbox

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// The sources microsandbox files a sandbox's log entries under.
const (
	sdkSourceStdout = "stdout"
	sdkSourceStderr = "stderr"
	sdkSourceOutput = "output"
	sdkSourceSystem = "system"
)

// mainSession is the session a main process's output is filed under.
//
// microsandbox numbers the exec sessions of one boot from 1, in the order it
// sees them asked for, and files what each says under its number. The engine
// starts an instance's main process before anything else may be exec'd into
// it, on every boot, so the first session of each boot is the main process.
const mainSession = 1

// logEntry is one entry of a sandbox's log as microsandbox reads it back: a
// chunk of what one source said, which may hold several lines or part of one.
type logEntry struct {
	source  string
	session *uint64
	at      time.Time
	data    []byte
}

// runtimeLine is what microsandbox's own runtime writes: its tracing, at a
// level and from a target, or a marker of the sandbox starting or stopping.
// Every other line of the system source is the guest's console.
var runtimeLine = regexp.MustCompile(`^\s*(?:(?:TRACE|DEBUG|INFO|WARN|ERROR)\s+[A-Za-z0-9_:]+:\s|--- sandbox \w+ ---)`)

// sourceOf is which of the engine's sources an entry is: what the runtime and
// the guest's console said are the kernel's and the runtime's, the main
// process's session is the main process's, and every other session is an
// exec's.
func sourceOf(entry logEntry, hasMain bool) string {
	if entry.source == sdkSourceSystem {
		if runtimeLine.Match(entry.data) {
			return vm.LogSourceRuntime
		}

		return vm.LogSourceKernel
	}

	if hasMain && entry.session != nil && *entry.session == mainSession {
		return vm.LogSourceMain
	}

	return vm.LogSourceExec
}

// linesOf is the lines a sandbox's log entries hold, in the order they were
// written. An entry is split at its newlines, and what a terminal ends a line
// with is not part of it. A line written in two chunks reads as two.
func linesOf(entries []logEntry, hasMain bool) []vm.LogLine {
	lines := make([]vm.LogLine, 0, len(entries))

	for _, entry := range entries {
		text := strings.TrimSuffix(string(entry.data), "\n")
		if len(entry.data) == 0 {
			continue
		}

		source := sourceOf(entry, hasMain)

		for line := range strings.SplitSeq(text, "\n") {
			lines = append(lines, vm.LogLine{
				At:     entry.at,
				Source: source,
				Line:   strings.TrimSuffix(line, "\r"),
			})
		}
	}

	return lines
}

// window is the lines options ask for: none written before Since, which keeps
// a line written at that very moment, and of those only the last Tail.
func window(lines []vm.LogLine, options vm.LogOptions) []vm.LogLine {
	kept := make([]vm.LogLine, 0, len(lines))

	for _, line := range lines {
		if options.Since.IsZero() || !line.At.Before(options.Since) {
			kept = append(kept, line)
		}
	}

	if options.Tail > 0 && uint(len(kept)) > options.Tail {
		kept = kept[uint(len(kept))-options.Tail:]
	}

	return kept
}

// merged is two runs of lines, each in the order it was written, as one.
func merged(lines []vm.LogLine, more []vm.LogLine) []vm.LogLine {
	all := append(slices.Clone(lines), more...)

	slices.SortStableFunc(all, func(a vm.LogLine, b vm.LogLine) int {
		return a.At.Compare(b.At)
	})

	return all
}
