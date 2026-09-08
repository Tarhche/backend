package container

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"time"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// logLineLimit caps a single line, so a container writing one enormous line
// without a newline cannot grow the buffer holding it without bound. Once the
// limit is reached, what has been read is emitted as a line of its own.
const logLineLimit = 64 * 1024

// StreamLogs follows a container's output from since onward, handing each line
// to emit as it arrives. It returns when the container's output ends, when emit
// refuses a line, or when ctx is done — which is how a caller stops following.
//
// Docker is asked to timestamp every line, so a caller can resume from where it
// left off and the lines it reads twice can be recognised as ones it already
// has.
func (m *DockerManager) StreamLogs(ctx context.Context, containerUUID string, since time.Time, emit func(container.LogLine) error) error {
	ctx, span := m.tracer.Start(ctx, "docker.container.logs.stream",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
	)
	defer span.End()

	options := containerTypes.LogsOptions{
		Follow:     true,
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
	}

	if !since.IsZero() {
		options.Since = since.Format(time.RFC3339Nano)
	}

	readCloser, err := m.client.ContainerLogs(ctx, containerUUID, options)
	if err != nil {
		return trace.RecordError(span, err)
	}
	defer readCloser.Close()

	// docker interleaves both streams down one connection and stdcopy pulls
	// them apart, writing to these one at a time. Emitting from inside Write is
	// what keeps a container's output in the order it was produced, with no
	// merging to do afterwards.
	stdout := &lineWriter{stream: container.StreamStdout, emit: emit}
	stderr := &lineWriter{stream: container.StreamStderr, emit: emit}

	_, copyErr := stdcopy.StdCopy(stdout, stderr, readCloser)

	// whatever a container wrote without a closing newline is still output.
	flushErr := errors.Join(stdout.flush(), stderr.flush())

	// a follow that ends because its context was cancelled is the caller having
	// stopped listening, not a failure.
	if ctx.Err() != nil {
		return nil
	}

	if err := errors.Join(stdout.err, stderr.err, flushErr); err != nil {
		return trace.RecordError(span, err)
	}

	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		return trace.RecordError(span, copyErr)
	}

	return nil
}

// lineWriter turns the bytes stdcopy hands it into whole lines and emits them.
type lineWriter struct {
	stream container.Stream
	emit   func(container.LogLine) error
	buffer bytes.Buffer
	err    error
}

var _ io.Writer = &lineWriter{}

// Write never reports a short write: refusing bytes would make stdcopy retry
// them, and the reason to stop is carried in err instead.
func (w *lineWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	w.buffer.Write(p)

	for {
		line, err := w.buffer.ReadString('\n')
		if err != nil {
			// an incomplete line goes back in the buffer to be finished by
			// what comes next, unless it has grown past what one line may be.
			w.buffer.Reset()
			w.buffer.WriteString(line)

			if w.buffer.Len() >= logLineLimit {
				if err := w.flush(); err != nil {
					return len(p), err
				}
			}

			return len(p), nil
		}

		if err := w.write(strings.TrimRight(line, "\r\n")); err != nil {
			return len(p), err
		}
	}
}

// flush emits whatever is left in the buffer as a line of its own.
func (w *lineWriter) flush() error {
	if w.buffer.Len() == 0 {
		return nil
	}

	line := w.buffer.String()
	w.buffer.Reset()

	return w.write(strings.TrimRight(line, "\r\n"))
}

func (w *lineWriter) write(line string) error {
	if w.err != nil {
		return w.err
	}

	at, content := splitTimestamp(line)

	w.err = w.emit(container.LogLine{Stream: w.stream, Content: content, At: at})

	return w.err
}

// splitTimestamp takes apart the "<rfc3339nano> <line>" docker writes when it
// is asked to timestamp its output. A line that somehow carries no timestamp is
// still worth keeping, so it is stamped with the time it was read.
func splitTimestamp(line string) (time.Time, string) {
	stamp, content, found := strings.Cut(line, " ")
	if !found {
		return time.Now().UTC(), line
	}

	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Now().UTC(), line
	}

	return at.UTC(), content
}
