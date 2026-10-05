// Package stackResult hears how the compose commands run on stacks went.
//
// A result is only taken for the command a stack is waiting on: a stack that
// was asked for something else since, or whose result was heard already, is
// not moved by a late or repeated one.
package stackResult

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
)

// defaultReason is said of a stack whose node did not say what went wrong.
const defaultReason = "the compose command failed"

// waitingOn is the compose command a stack in state is waiting on.
func waitingOn(state stack.State) (stack.Action, bool) {
	switch state {
	case stack.Deploying:
		return stack.ActionUp, true
	case stack.Starting:
		return stack.ActionStart, true
	case stack.Stopping:
		return stack.ActionStop, true
	case stack.Restarting:
		return stack.ActionRestart, true
	case stack.Removing:
		return stack.ActionDown, true
	}

	return "", false
}

// tail is the end of what a compose command printed, as much of it as a stack
// keeps: the last lines are the ones that say what went wrong.
func tail(output string) string {
	if len(output) <= stack.MaxOutput {
		return output
	}

	return output[len(output)-stack.MaxOutput:]
}

// StackCompleted writes down that a compose command on a stack succeeded, and
// takes away the record of one that was taken down.
type StackCompleted struct {
	stackRepository stack.Repository
	logger          *slog.Logger
}

var _ domain.MessageHandler = &StackCompleted{}

func NewStackCompleted(stackRepository stack.Repository, logger *slog.Logger) *StackCompleted {
	return &StackCompleted{stackRepository: stackRepository, logger: logger}
}

// Handle never fails for a message that will never be handled: redelivering it
// would only fail the same way, at once and for ever.
func (h *StackCompleted) Handle(ctx context.Context, data []byte) error {
	var completed events.StackCompleted
	if err := json.Unmarshal(data, &completed); err != nil {
		h.logger.ErrorContext(ctx, "a stack result that cannot be read", "error", err)

		return nil
	}

	s, err := h.stackRepository.GetOne(ctx, completed.StackUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	if waiting, ok := waitingOn(s.State); !ok || waiting != completed.Action {
		return nil
	}

	if completed.Action == stack.ActionDown {
		return h.stackRepository.Delete(ctx, s.UUID)
	}

	s.State = stack.Running
	if completed.Action == stack.ActionStop {
		s.State = stack.Stopped
	}

	s.Reason = ""
	s.Output = tail(completed.Output)

	_, err = h.stackRepository.Save(ctx, &s)

	return err
}

// StackFailed writes down that a compose command on a stack failed, with what
// compose printed before it gave up, which usually says more than the reason.
type StackFailed struct {
	stackRepository stack.Repository
	logger          *slog.Logger
}

var _ domain.MessageHandler = &StackFailed{}

func NewStackFailed(stackRepository stack.Repository, logger *slog.Logger) *StackFailed {
	return &StackFailed{stackRepository: stackRepository, logger: logger}
}

// Handle never fails for a message that will never be handled: redelivering it
// would only fail the same way, at once and for ever.
func (h *StackFailed) Handle(ctx context.Context, data []byte) error {
	var failed events.StackFailed
	if err := json.Unmarshal(data, &failed); err != nil {
		h.logger.ErrorContext(ctx, "a stack failure that cannot be read", "error", err)

		return nil
	}

	s, err := h.stackRepository.GetOne(ctx, failed.StackUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	if waiting, ok := waitingOn(s.State); !ok || waiting != failed.Action {
		return nil
	}

	h.logger.WarnContext(ctx, "a compose command failed", "stack", s.UUID, "action", failed.Action, "node", failed.NodeName, "reason", failed.Reason)

	s.State = stack.Failed
	s.Reason = failed.Reason
	if len(s.Reason) == 0 {
		s.Reason = defaultReason
	}

	s.Output = tail(failed.Output)

	_, err = h.stackRepository.Save(ctx, &s)

	return err
}
