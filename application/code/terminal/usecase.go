// Package terminal gives a reader a way into the container their snippet is
// running in.
//
// It is the same terminal the dashboard opens, on a container that holds
// nothing but the code on the page: a job the code runner started, owned by
// nobody, taken away when it ends. Anything else — a container somebody is
// running from the dashboard, above all — is not there as far as this is
// concerned, whatever uuid is named.
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/code/runCode"
	"github.com/khanzadimahdi/testproject/application/runner/terminal"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type UseCase struct {
	runner    runnerManager.Client
	validator domain.Validator

	// sessions holds the terminals this replica has open, and pumps what they
	// write back to whoever is watching.
	sessions *terminal.Sessions

	logger *slog.Logger
}

var _ domain.MessageHandler = &UseCase{}

func NewUseCase(
	runner runnerManager.Client,
	validator domain.Validator,
	sessions *terminal.Sessions,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		runner:    runner,
		validator: validator,
		sessions:  sessions,
		logger:    logger,
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

	c, err := uc.runner.Container(ctx, request.ContainerUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.sessions.Fail(ctx, request.ID, domain.ValidationErrors{"container_uuid": "not_exists"})
	} else if err != nil {
		return err
	}

	// a snippet's container, and nothing else: a job the code runner started,
	// which belongs to nobody. A container from the dashboard belongs to
	// somebody, and this is not the way to it.
	if c.Kind != task.KindJob || c.OwnerUUID != runCode.CodeRunnerOwnerUUID {
		uc.logger.WarnContext(ctx, "a terminal was asked for on a container the code runner does not own", "container", request.ContainerUUID)

		return uc.sessions.Fail(ctx, request.ID, domain.ValidationErrors{"container_uuid": "not_exists"})
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

// InputHandler is the half of this use case that takes what a reader types.
func (uc *UseCase) InputHandler() domain.MessageHandler {
	return domain.MessageHandlerFunc(func(ctx context.Context, data []byte) error {
		return uc.sessions.Write(ctx, data)
	})
}
