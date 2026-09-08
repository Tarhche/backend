package container

import (
	"context"
	"time"
)

// Stream tells which of the container's two output streams a line came from.
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

// LogLine is one line a container wrote, as it was written.
type LogLine struct {
	Stream  Stream
	Content string
	At      time.Time
}

// Log is a line kept against the task that produced it. It outlives the
// container: lines are held until the task itself is deleted, which is what
// lets the dashboard show a container's whole history rather than whatever
// docker still happens to hold.
type Log struct {
	TaskUUID    string
	ContainerID string

	LogLine
}

// LogRepository stores the lines containers write.
//
// Append is idempotent: a worker that reconnects to a container's log stream
// resumes from a timestamp it has already shipped, so the same line may arrive
// more than once and must be stored only once.
type LogRepository interface {
	Append(ctx context.Context, logs []Log) error
	Get(ctx context.Context, taskUUID string, after time.Time, limit uint) ([]Log, error)
	DeleteByTask(ctx context.Context, taskUUID string) error
}
