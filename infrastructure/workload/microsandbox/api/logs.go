package api

import "time"

// LogLine is one line a run's main process wrote. RouteRunLogs streams them as
// newline-delimited JSON, one on each line, flushed as each is written.
//
// At is unique and strictly increasing within a run: the service stamps each
// line with the later of the time it read it and a nanosecond after the line
// before. A client that lost its stream asks again from the last At it kept,
// and since QuerySince is inclusive, as docker's is, it gets that one line
// again, recognisable by its At, followed by every line it missed.
//
// With QueryFollow the stream stays open while the run is running, stopping
// or restarting. It ends once the run has ended and every line it wrote has
// been sent, or when the client goes away.
type LogLine struct {
	// Stream is StreamStdout or StreamStderr.
	Stream string    `json:"stream"`
	At     time.Time `json:"at"`

	// Content is the line without its newline, and at most MaxLogContent
	// bytes. It is a JSON string, so bytes that are not UTF-8 arrive as
	// U+FFFD.
	Content string `json:"content"`
}

// The streams a LogLine comes from.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

// MaxLogContent is the most a LogLine's Content holds, in bytes.
const MaxLogContent = 64 << 10
