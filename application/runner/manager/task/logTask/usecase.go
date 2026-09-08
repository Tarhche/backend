package logTask

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// TaskLogged stores the lines a worker ships as its containers write them.
//
// A worker that reconnects to a container's stream resumes from a timestamp it
// has already shipped, so the same lines arrive twice; the repository
// recognises them by their own content and stores each one once.
type TaskLogged struct {
	logRepository container.LogRepository

	// maxBytes caps what one container may keep, so a chatty container cannot
	// fill the disk. Past it, its lines are dropped rather than stored.
	maxBytes int64

	logger *slog.Logger
}

var _ domain.MessageHandler = &TaskLogged{}

func NewTaskLogged(logRepository container.LogRepository, maxBytes int64, logger *slog.Logger) *TaskLogged {
	return &TaskLogged{
		logRepository: logRepository,
		maxBytes:      maxBytes,
		logger:        logger,
	}
}

func (uc *TaskLogged) Handle(ctx context.Context, data []byte) error {
	var logged events.TaskLogged
	if err := json.Unmarshal(data, &logged); err != nil {
		// a malformed batch is not worth redelivering.
		uc.logger.WarnContext(ctx, "dropping a malformed log batch", "error", err)

		return nil
	}

	if len(logged.Lines) == 0 || len(logged.UUID) == 0 {
		return nil
	}

	if uc.overCap(ctx, logged.UUID) {
		return nil
	}

	logs := make([]container.Log, len(logged.Lines))
	for i, line := range logged.Lines {
		logs[i] = container.Log{
			TaskUUID:    logged.UUID,
			ContainerID: logged.ContainerUUID,
			LogLine: container.LogLine{
				Stream:  container.Stream(line.Stream),
				Content: line.Content,
				At:      line.At,
			},
		}
	}

	return uc.logRepository.Append(ctx, logs)
}

// sizer is the part of a log repository that can report what a task has stored.
// A repository that cannot say is simply never over its cap.
type sizer interface {
	Size(ctx context.Context, taskUUID string) (int64, error)
}

func (uc *TaskLogged) overCap(ctx context.Context, taskUUID string) bool {
	if uc.maxBytes <= 0 {
		return false
	}

	sizer, ok := uc.logRepository.(sizer)
	if !ok {
		return false
	}

	size, err := sizer.Size(ctx, taskUUID)
	if err != nil {
		uc.logger.WarnContext(ctx, "could not measure a container's log", "error", err, "taskUUID", taskUUID)

		return false
	}

	if size < uc.maxBytes {
		return false
	}

	uc.logger.WarnContext(ctx, "a container has reached its log limit, dropping further lines", "taskUUID", taskUUID, "bytes", size)

	return true
}
