// Package attachContainer gives the dashboard a terminal inside a running
// container.
//
// One request opens it, and the reply to that request is a stream: everything
// the command writes, chunk by chunk, until it ends. What the person types
// comes back on a second subject naming that same stream, so a terminal is one
// conversation rather than a connection of its own.
package attachContainer

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

// chunkSize is how much of a command's output is carried in one reply.
const chunkSize = 4 << 10

// UseCase opens terminals and keeps them until their client goes away.
type UseCase struct {
	runner        runnerManager.Client
	authenticator *auth.Authenticator
	authorizer    domain.Authorizer
	containers    *runnerAccess.Containers
	validator     domain.Validator
	replyer       domain.Replyer

	// streams is what tells this replica that a client has stopped listening,
	// so the terminal it was holding is closed rather than left running.
	streams   *gateway.Streams
	terminals *terminals

	logger *slog.Logger
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
		terminals:     newTerminals(),
		logger:        logger,
	}
}

// Handle opens a terminal and streams its output back until it ends.
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

	// a terminal is the strongest thing the dashboard offers — it is a shell
	// inside somebody's container — so it has a permission of its own, and a
	// container that is not this person's is not one they may open.
	// looked up as far as this person may reach: a container that is not
	// theirs is not there for them.
	if _, err := uc.containers.Of(ctx, user.UUID, request.ContainerUUID, permission.RunnerContainersAttach, permission.SelfRunnerContainersAttach); err != nil {
		if errors.Is(err, domain.ErrNotExists) {
			return uc.fail(ctx, request.ID, domain.ValidationErrors{"container_uuid": "not_exists"})
		}

		return err
	}

	attachment, err := uc.runner.AttachContainer(ctx, request.ContainerUUID, request.Command)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.fail(ctx, request.ID, domain.ValidationErrors{"container_uuid": "not_exists"})
	} else if err != nil {
		return err
	}

	uc.serve(ctx, request.ID, attachment)

	return nil
}

// serve pumps a terminal's output back to its client until one end stops.
func (uc *UseCase) serve(ctx context.Context, requestID string, attachment runnerManager.Attachment) {
	// detached from the message that opened it: a terminal lives as long as
	// the person using it, not as long as the request that asked for one.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	uc.terminals.add(requestID, attachment)
	uc.streams.Add(requestID, cancel)

	go func() {
		defer func() {
			uc.streams.Remove(requestID)
			uc.terminals.remove(requestID)
			_ = attachment.Close()
			cancel()

			uc.end(ctx, requestID)
		}()

		// closing the attachment is what releases a read parked on it, so a
		// cancelled context has to reach it that way.
		stop := context.AfterFunc(ctx, func() { _ = attachment.Close() })
		defer stop()

		buffer := make([]byte, chunkSize)

		for {
			n, err := attachment.Read(buffer)

			if n > 0 {
				// the bytes are the payload: a terminal's output is not JSON,
				// and a reply carries it to the client as it stands.
				chunk := make([]byte, n)
				copy(chunk, buffer[:n])

				if err := uc.replyer.Reply(ctx, &domain.Reply{
					RequestID: requestID,
					Kind:      domain.ReplyChunk,
					Payload:   chunk,
				}); err != nil {
					uc.logger.ErrorContext(ctx, "error on writing terminal output", "error", err)

					return
				}
			}

			if err != nil {
				if !errors.Is(err, io.EOF) {
					uc.logger.WarnContext(ctx, "a terminal ended", "error", err)
				}

				return
			}
		}
	}()
}

// HandleInput carries what somebody typed, or the size of their window, to the
// terminal they have open.
func (uc *UseCase) HandleInput(ctx context.Context, data []byte) error {
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return nil
	}

	attachment, ok := uc.terminals.get(input.ID)
	if !ok {
		// the terminal has ended, or was opened on another replica. Either way
		// there is nothing here to write to, and redelivering would not find
		// it.
		uc.logger.WarnContext(ctx, "input for a terminal this replica does not hold", "streamID", input.ID)

		return nil
	}

	switch input.Type {
	case inputResize:
		if input.Rows == 0 || input.Cols == 0 {
			return nil
		}

		return attachment.Resize(ctx, input.Rows, input.Cols)

	case inputKeys:
		if len(input.Data) == 0 {
			return nil
		}

		_, err := attachment.Write(input.Data)

		return err

	default:
		return nil
	}
}

// end tells the client that its terminal is over.
func (uc *UseCase) end(ctx context.Context, requestID string) {
	if err := uc.replyer.Reply(ctx, &domain.Reply{RequestID: requestID, Kind: domain.ReplyEOF}); err != nil {
		uc.logger.ErrorContext(ctx, "error on ending a terminal", "error", err)
	}
}

// fail answers a request that never opened a terminal, and ends it, so the
// client is not left waiting on a stream that will never start.
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

// InputHandler is the half of this use case that takes what a client types.
func (uc *UseCase) InputHandler() domain.MessageHandler {
	return domain.MessageHandlerFunc(uc.HandleInput)
}
