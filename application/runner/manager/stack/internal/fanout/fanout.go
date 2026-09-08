// Package fanout applies a stack-wide command to each of the services in it.
package fanout

import (
	"context"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Over runs command against every service of a stack.
//
// A service that cannot take the command — one already stopped when the stack
// is stopped, or gone entirely — is passed over rather than failing the whole
// stack: what was asked for was that the stack end up in that state, and a
// service already in it is not a reason to leave the others alone. Anything
// else stops the fan-out, because it means the runner could not do the work.
func Over(ctx context.Context, services []task.Task, logger *slog.Logger, command func(context.Context, string) error) error {
	for _, service := range services {
		err := command(ctx, service.UUID)

		switch {
		case err == nil:
		case errors.Is(err, domain.ErrNotExists):
			logger.WarnContext(ctx, "a service of the stack no longer exists", "service", service.UUID)
		default:
			return err
		}
	}

	return nil
}
