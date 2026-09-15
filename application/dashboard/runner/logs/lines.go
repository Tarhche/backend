package logs

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// Lines is a task saying what it has just written, which is where a
// follower gets everything after the backlog it opened with.
func (f *Followers) Lines(ctx context.Context, data []byte) error {
	if f.Len() == 0 {
		return nil
	}

	var logged taskEvents.TaskLogged
	if err := json.Unmarshal(data, &logged); err != nil {
		return nil
	}

	followers := f.of(logged.UUID)
	if len(followers) == 0 {
		return nil
	}

	lines := make([]task.LogLine, len(logged.Lines))
	for i, line := range logged.Lines {
		lines[i] = task.LogLine{
			Stream:  task.Stream(line.Stream),
			Content: line.Content,
			At:      line.At,
		}
	}

	for _, follower := range followers {
		follower.write(ctx, lines)
	}

	return nil
}

// Deleted ends the follows of a task that is gone: there is nothing left
// to write anything more.
func (f *Followers) Deleted(ctx context.Context, data []byte) error {
	if f.Len() == 0 {
		return nil
	}

	var deleted taskEvents.TaskDeleted
	if err := json.Unmarshal(data, &deleted); err != nil {
		return nil
	}

	for _, follower := range f.of(deleted.UUID) {
		f.Remove(follower.requestID)

		if err := f.replyer.Reply(ctx, &domain.Reply{RequestID: follower.requestID, Kind: domain.ReplyEOF}); err != nil {
			f.logger.ErrorContext(ctx, "error on ending a log stream", "error", err, "requestID", follower.requestID)
		}
	}

	return nil
}
