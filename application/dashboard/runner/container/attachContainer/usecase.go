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
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/auth"
	runnerAccess "github.com/khanzadimahdi/testproject/application/dashboard/runner/access"
	"github.com/khanzadimahdi/testproject/application/runner/terminal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// UseCase opens terminals and keeps them until their client goes away.
type UseCase struct {
	runner        runnerManager.Client
	authenticator *auth.Authenticator
	authorizer    domain.Authorizer
	containers    *runnerAccess.Containers
	validator     domain.Validator

	// sessions holds the terminals this replica has open, and pumps what they
	// write back to whoever is watching.
	sessions *terminal.Sessions

	logger *slog.Logger
}

var _ domain.MessageHandler = &UseCase{}

func NewUseCase(
	runner runnerManager.Client,
	authenticator *auth.Authenticator,
	authorizer domain.Authorizer,
	containers *runnerAccess.Containers,
	validator domain.Validator,
	sessions *terminal.Sessions,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		runner:        runner,
		authenticator: authenticator,
		authorizer:    authorizer,
		containers:    containers,
		validator:     validator,
		sessions:      sessions,
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
		return uc.sessions.Fail(ctx, request.ID, validationErrors)
	}

	user, err := uc.authenticator.Authenticate(ctx, request.AccessToken)
	if err != nil {
		return uc.sessions.Fail(ctx, request.ID, domain.ValidationErrors{"access_token": "unauthenticated"})
	}

	// a terminal is the strongest thing the dashboard offers — it is a shell
	// inside somebody's container — so it has a permission of its own, and a
	// container that is not this person's is not one they may open.
	// looked up as far as this person may reach: a container that is not
	// theirs is not there for them.
	if _, err := uc.containers.Of(ctx, user.UUID, request.ContainerUUID, permission.RunnerContainersAttach, permission.SelfRunnerContainersAttach); err != nil {
		if errors.Is(err, domain.ErrNotExists) {
			return uc.sessions.Fail(ctx, request.ID, domain.ValidationErrors{"container_uuid": "not_exists"})
		}

		return err
	}

	attachment, err := uc.runner.AttachContainer(ctx, request.ContainerUUID, request.Command)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.sessions.Fail(ctx, request.ID, domain.ValidationErrors{"container_uuid": "not_exists"})
	} else if err != nil {
		return err
	}

	uc.sessions.Serve(ctx, request.ID, attachment)

	return nil
}

// InputHandler is the half of this use case that takes what a client types.
func (uc *UseCase) InputHandler() domain.MessageHandler {
	return domain.MessageHandlerFunc(func(ctx context.Context, data []byte) error {
		return uc.sessions.Write(ctx, data)
	})
}
