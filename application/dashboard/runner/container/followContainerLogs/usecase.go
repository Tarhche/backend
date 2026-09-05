// Package followContainerLogs streams a container's output to the dashboard as
// it is written.
//
// One request opens the stream and the reply to it is that stream: every line
// from the moment asked for onward, until the container stops writing or the
// client says it has seen enough.
package followContainerLogs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/auth"
	runnerAccess "github.com/khanzadimahdi/testproject/application/dashboard/runner/access"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

// FollowName is the subject a client opens a container's log on.
const FollowName = "runnerContainerLogs"

// UseCase follows containers' logs on behalf of the clients watching them.
type UseCase struct {
	runner        runnerManager.Client
	authenticator *auth.Authenticator
	authorizer    domain.Authorizer
	containers    *runnerAccess.Containers
	validator     domain.Validator
	replyer       domain.Replyer
	streams       *gateway.Streams
	logger        *slog.Logger
}

var _ domain.MessageHandler = &UseCase{}

func NewUseCase(
	runner runnerManager.Client,
	authenticator *auth.Authenticator,
	authorizer domain.Authorizer,
	containers *runnerAccess.Containers,
	validator domain.Validator,
	replyer domain.Replyer,
	streams *gateway.Streams,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		runner:        runner,
		authenticator: authenticator,
		authorizer:    authorizer,
		containers:    containers,
		validator:     validator,
		replyer:       replyer,
		streams:       streams,
		logger:        logger,
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

	user, err := uc.authenticator.Authenticate(ctx, request.AccessToken)
	if err != nil {
		return uc.fail(ctx, request.ID, domain.ValidationErrors{"access_token": "unauthenticated"})
	}

	// looked up as far as this person may reach: a container that is not
	// theirs is not there for them.
	if _, err := uc.containers.Of(ctx, user.UUID, request.ContainerUUID, permission.RunnerContainersLogs, permission.SelfRunnerContainersLogs); err != nil {
		if errors.Is(err, domain.ErrNotExists) {
			return uc.fail(ctx, request.ID, domain.ValidationErrors{"container_uuid": "not_exists"})
		}

		return err
	}

	stream, err := uc.runner.FollowContainerLogs(ctx, request.ContainerUUID, request.After)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.fail(ctx, request.ID, domain.ValidationErrors{"container_uuid": "not_exists"})
	} else if err != nil {
		return err
	}

	uc.serve(ctx, request.ID, stream)

	return nil
}

// serve carries a container's lines to the client watching them.
func (uc *UseCase) serve(ctx context.Context, requestID string, stream runnerManager.LogStream) {
	// detached from the message that asked for it: the stream lives as long as
	// the person watching, not as long as the request.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	uc.streams.Add(requestID, cancel)

	go func() {
		defer func() {
			uc.streams.Remove(requestID)
			_ = stream.Close()
			cancel()

			uc.end(ctx, requestID)
		}()

		for {
			line, err := stream.Next(ctx)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					uc.logger.WarnContext(ctx, "a log stream ended", "error", err)
				}

				return
			}

			payload, err := json.Marshal(LineResponse{
				Stream:  line.Stream.String(),
				Content: line.Content,
				At:      line.At,
			})
			if err != nil {
				uc.logger.ErrorContext(ctx, "error on marshalling a log line", "error", err)

				return
			}

			if err := uc.replyer.Reply(ctx, &domain.Reply{
				RequestID: requestID,
				Kind:      domain.ReplyChunk,
				Payload:   payload,
			}); err != nil {
				uc.logger.ErrorContext(ctx, "error on writing a log line", "error", err)

				return
			}
		}
	}()
}

func (uc *UseCase) end(ctx context.Context, requestID string) {
	if err := uc.replyer.Reply(ctx, &domain.Reply{RequestID: requestID, Kind: domain.ReplyEOF}); err != nil {
		uc.logger.ErrorContext(ctx, "error on ending a log stream", "error", err)
	}
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
