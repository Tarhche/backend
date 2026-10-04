//go:build microsandbox

package sdk

import (
	"context"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

// execStream is the SDK's exec handle as a stream.
type execStream struct {
	handle *msb.ExecHandle
}

var _ stream = execStream{}

func (s execStream) recv(ctx context.Context) (item, error) {
	event, err := s.handle.Recv(ctx)
	if err != nil {
		return item{}, err
	}

	return itemOf(event), nil
}

func (s execStream) signal(ctx context.Context, signal int) error {
	return s.handle.Signal(ctx, signal)
}

func (s execStream) resize(ctx context.Context, rows, cols uint16) error {
	return s.handle.Resize(ctx, rows, cols)
}

func (s execStream) kill(ctx context.Context) error {
	return s.handle.Kill(ctx)
}

// close lets go of the handle, and microsandbox ends the command with it.
func (s execStream) close() error {
	return s.handle.Close()
}

// itemOf is an exec event in the port's terms. A command a signal ended
// exits -1, whichever signal it was and whoever sent it, and that is handed on
// as it is: the supervisor knows what it sent. A command that never started
// fails with the errno the guest agent reports.
func itemOf(event *msb.ExecEvent) item {
	switch event.Kind {
	case msb.ExecEventStarted:
		return item{event: runs.Event{Kind: runs.EventStarted}}
	case msb.ExecEventStdout:
		return item{event: runs.Event{Kind: runs.EventStdout, Data: event.Data}}
	case msb.ExecEventStderr:
		return item{event: runs.Event{Kind: runs.EventStderr, Data: event.Data}}
	case msb.ExecEventExited:
		return item{event: runs.Event{Kind: runs.EventExited, ExitCode: event.ExitCode}}
	case msb.ExecEventFailed:
		failed := runs.Event{Kind: runs.EventFailed}
		if failure := event.Failure; failure != nil {
			failed.Errno = errnoName(failure.ErrnoName, failure.Errno, failure.Kind)
			failed.Message = failure.Message
		}

		return item{event: failed}
	case msb.ExecEventDone:
		return item{done: true}
	default:
		// a write to stdin that failed, which the next write reports
		// anyway, or a kind of a later version
		return item{skip: true}
	}
}
