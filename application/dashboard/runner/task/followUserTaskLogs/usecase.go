// Package followUserTaskLogs streams one of somebody's own tasks'
// output to the dashboard as it is written.
//
// One request opens the stream and the reply to it is that stream: what the
// task had already written from the moment asked for, and then every line
// as it arrives, until the task is taken away or the client says it has
// seen enough.
//
// It is asked for as theirs, so a task that is somebody else's is not
// found rather than refused: to whoever may only follow their own, one that is
// not theirs and one that does not exist are the same thing.
package followUserTaskLogs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/logs"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

// FollowName is the subject a client opens one of its own tasks' log on.
const FollowName = "runnerUserTaskLogs"

// backlog is how much of what a task has already written a stream opens
// with, which is the same page a log is read by.
const backlog uint = 500

// UseCase follows tasks' logs on behalf of the people who own them.
type UseCase struct {
	runner    runnerManager.Client
	followers *logs.Followers
	validator domain.Validator
	replyer   domain.Replyer
	streams   *gateway.Streams
	logger    *slog.Logger
}

var _ domain.MessageHandler = &UseCase{}

func NewUseCase(
	runner runnerManager.Client,
	followers *logs.Followers,
	validator domain.Validator,
	replyer domain.Replyer,
	streams *gateway.Streams,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		runner:    runner,
		followers: followers,
		validator: validator,
		replyer:   replyer,
		streams:   streams,
		logger:    logger,
	}
}

func (uc *UseCase) Handle(ctx context.Context, data []byte) error {
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return nil
	}

	if validationErrors := uc.validator.Validate(&request); len(validationErrors) > 0 {
		return uc.fail(ctx, request.ID, validationErrors)
	}

	if _, err := uc.runner.TaskOf(ctx, auth.UUIDFromContext(ctx), request.TaskUUID); err != nil {
		if errors.Is(err, domain.ErrNotExists) {
			return uc.fail(ctx, request.ID, domain.ValidationErrors{"task_uuid": "not_exists"})
		}

		return err
	}

	// registered before the backlog is read, so a line written while it is
	// being read is kept rather than missed.
	follower := uc.followers.Follow(request.ID, request.TaskUUID, request.After)

	// the stream lasts as long as the client does: a cancellation from
	// whichever replica it walked away from is what ends it.
	uc.streams.Add(request.ID, func() { uc.followers.Remove(request.ID) })

	written, err := uc.runner.TaskLogs(ctx, request.TaskUUID, request.After, backlog)
	if err != nil {
		uc.streams.Remove(request.ID)
		uc.followers.Remove(request.ID)

		if errors.Is(err, domain.ErrNotExists) {
			return uc.fail(ctx, request.ID, domain.ValidationErrors{"task_uuid": "not_exists"})
		}

		return err
	}

	follower.CatchUp(ctx, written)

	return nil
}

func (uc *UseCase) fail(ctx context.Context, requestID string, validationErrors domain.ValidationErrors) error {
	payload, err := json.Marshal(map[string]any{"errors": validationErrors})
	if err != nil {
		return err
	}

	return uc.replyer.Reply(ctx, &domain.Reply{
		RequestID: requestID,
		Kind:      domain.ReplyEOF,
		Payload:   payload,
	})
}
