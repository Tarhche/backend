package task

import (
	"context"
	"time"
)

// Stream tells which of the task's two output streams a line came from.
type Stream uint8

const (
	StreamStdout Stream = 1
	StreamStderr Stream = 2
)

func (s Stream) String() string {
	switch s {
	case StreamStdout:
		return "stdout"
	case StreamStderr:
		return "stderr"
	default:
		return "unknown"
	}
}

// LogLine is one line a task wrote, as it was written.
type LogLine struct {
	Stream  Stream
	Content string
	At      time.Time
}

// Log is a line kept against the task that produced it. It outlives the
// task: lines are held until the task itself is deleted, which is what
// lets the dashboard show a task's whole history rather than whatever
// docker still happens to hold.
type Log struct {
	TaskUUID    string
	ExecutionID string

	LogLine
}

// LogRepository stores the lines tasks write.
//
// Append is idempotent: an orchestrator that reconnects to a task's log stream
// resumes from a timestamp it has already shipped, so the same line may arrive
// more than once and must be stored only once.
type LogRepository interface {
	Append(ctx context.Context, logs []Log) error
	Get(ctx context.Context, taskUUID string, after time.Time, limit uint) ([]Log, error)
	DeleteByTask(ctx context.Context, taskUUID string) error
}
