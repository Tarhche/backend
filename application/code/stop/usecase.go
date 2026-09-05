// Package stop takes away the container a snippet is running in.
//
// Stopping a snippet is removing it: there is nothing in one worth keeping,
// and running it again is a new container running the code as it is written
// now. Like the terminal a snippet offers, this reaches a job the code runner
// started and nothing else, whatever uuid is named.
package stop

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/code/runCode"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type UseCase struct {
	runner    runnerManager.Client
	validator domain.Validator
	replyer   domain.Replyer

	logger *slog.Logger
}

var _ domain.MessageHandler = &UseCase{}

func NewUseCase(
	runner runnerManager.Client,
	validator domain.Validator,
	replyer domain.Replyer,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		runner:    runner,
		validator: validator,
		replyer:   replyer,
		logger:    logger,
	}
}

func (uc *UseCase) Handle(ctx context.Context, data []byte) error {
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return nil
	}

	if validationErrors := uc.validator.Validate(&request); len(validationErrors) > 0 {
		return uc.reply(ctx, request.ID, &Response{ValidationErrors: validationErrors})
	}

	c, err := uc.runner.Container(ctx, request.ContainerUUID)
	if errors.Is(err, domain.ErrNotExists) {
		// a container that is already gone is what was asked for.
		return uc.reply(ctx, request.ID, &Response{})
	} else if err != nil {
		return err
	}

	// a snippet's container, and nothing else: a job the code runner started,
	// which belongs to nobody. A container from the dashboard belongs to
	// somebody, and this is not the way to it.
	if c.Kind != task.KindJob || c.OwnerUUID != runCode.CodeRunnerOwnerUUID {
		uc.logger.WarnContext(ctx, "a stop was asked for on a container the code runner does not own", "container", request.ContainerUUID)

		return uc.reply(ctx, request.ID, &Response{
			ValidationErrors: domain.ValidationErrors{"container_uuid": "not_exists"},
		})
	}

	if err := uc.runner.DeleteContainer(ctx, request.ContainerUUID); err != nil && !errors.Is(err, domain.ErrNotExists) {
		return err
	}

	return uc.reply(ctx, request.ID, &Response{})
}

func (uc *UseCase) reply(ctx context.Context, requestID string, response *Response) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}

	return uc.replyer.Reply(ctx, &domain.Reply{
		RequestID: requestID,
		Payload:   payload,
	})
}
