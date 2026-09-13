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

// StopName is what a reader asks on to have the task their snippet is
// running in taken away. Running the snippet again starts a new one.
const StopName = "codeStop"

type UseCase struct {
	runner    runnerManager.Client
	validator domain.Validator
	replyer   domain.Replyer

	logger *slog.Logger
}

// Eunsure UseCase implements the MessageHandler interface.
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

	c, err := uc.runner.Task(ctx, request.TaskUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.reply(ctx, request.ID, &Response{})
	} else if err != nil {
		return err
	}

	if c.Kind != task.KindJob || c.OwnerUUID != runCode.CodeRunnerOwnerUUID {
		uc.logger.WarnContext(ctx, "a stop was asked for on a task the code runner does not own", "task", request.TaskUUID)

		return uc.reply(ctx, request.ID, &Response{
			ValidationErrors: domain.ValidationErrors{"task_uuid": "not_exists"},
		})
	}

	if err := uc.runner.DeleteTask(ctx, request.TaskUUID); err != nil && !errors.Is(err, domain.ErrNotExists) {
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
