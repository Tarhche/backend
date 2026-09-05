// Package access finds the container somebody asked for, as far as they may
// reach.
//
// It is for the requests that arrive over a websocket rather than a route: they
// name a container without saying which listing it came from, so which of the
// two permissions they hold is what decides how the container is looked up —
// as anybody's, or as their own. One that is not theirs is not found, the same
// answer as one that was never there.
package access

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type Containers struct {
	runner     runnerManager.Client
	authorizer domain.Authorizer
}

func NewContainers(runner runnerManager.Client, authorizer domain.Authorizer) *Containers {
	return &Containers{runner: runner, authorizer: authorizer}
}

// Of is the container, looked up the way this person may look one up.
func (c *Containers) Of(ctx context.Context, userUUID string, containerUUID string, all string, own string) (task.Task, error) {
	mayAll, err := c.authorizer.Authorize(ctx, userUUID, all)
	if err != nil {
		return task.Task{}, err
	}

	if mayAll {
		return c.runner.Container(ctx, containerUUID)
	}

	mayOwn, err := c.authorizer.Authorize(ctx, userUUID, own)
	if err != nil {
		return task.Task{}, err
	}

	if !mayOwn {
		return task.Task{}, domain.ErrNotExists
	}

	return c.runner.ContainerOf(ctx, userUUID, containerUUID)
}
