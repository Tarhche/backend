package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// maxLogLine bounds one line of a run's log as it travels. Its content is at
// most api.MaxLogContent, and written as JSON a line can grow to several times
// that, but a line past this is not one the service sent.
const maxLogLine = 1 << 20

// Logs writes everything a run's main process has written so far, both streams
// together in the order they were written, a line at a time.
func (r *Runtime) Logs(ctx context.Context, runID string, writer io.Writer) error {
	ctx, span := r.client.span(ctx, "task.logs", attribute.String("task.id", runID))
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, answerTimeout)
	defer cancel()

	err := r.readLog(ctx, runID, nil, func(line api.LogLine) error {
		_, err := io.WriteString(writer, line.Content+"\n")

		return err
	})

	return trace.RecordError(span, err)
}

// StreamLogs follows a run's log from since on, handing each line to emit as it
// is written. It returns once the run has ended and every line it wrote has
// been handed over, with emit's error when emit refuses a line, or when ctx is
// done, which is how a caller stops following.
//
// Since is inclusive, as docker's is, and the service stamps every line of a
// run with a time of its own, so a caller that resumes from the last line it
// kept gets that line again, recognisable by its time, and then everything it
// missed.
func (r *Runtime) StreamLogs(ctx context.Context, runID string, since time.Time, emit func(task.LogLine) error) error {
	ctx, span := r.client.span(ctx, "task.logs.stream", attribute.String("task.id", runID))
	defer span.End()

	query := url.Values{api.QueryFollow: {"true"}}
	if !since.IsZero() {
		query.Set(api.QuerySince, since.UTC().Format(time.RFC3339Nano))
	}

	err := r.readLog(ctx, runID, query, func(line api.LogLine) error {
		return emit(task.LogLine{
			Stream:  stream(line.Stream),
			Content: line.Content,
			At:      line.At.UTC(),
		})
	})

	// a follow that ends because its context was done is the caller having
	// stopped listening, not a failure.
	if ctx.Err() != nil {
		return nil
	}

	return trace.RecordError(span, err)
}

// readLog reads a run's log as the service streams it, one LogLine on each
// line, and hands each to each until the stream ends or each refuses one.
func (r *Runtime) readLog(ctx context.Context, runID string, query url.Values, each func(api.LogLine) error) error {
	response, err := r.client.do(ctx, request{
		route:     api.RouteRunLogs,
		wildcards: runPath(runID),
		query:     query,
		accept:    api.ContentTypeNDJSON,
	})
	if err != nil {
		return err
	}
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, api.MaxLogContent), maxLogLine)

	for scanner.Scan() {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}

		var line api.LogLine
		if err := json.Unmarshal(raw, &line); err != nil {
			return fmt.Errorf("workload-microsandbox sent a log line that could not be read: %w", err)
		}

		if err := each(line); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("a run's log stream broke off: %w", err)
	}

	return nil
}
