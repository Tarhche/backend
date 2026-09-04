// Package access decides what somebody may do to one container or one stack.
//
// The runner keeps who asked for each of them; the permissions come in pairs,
// one over everybody's and one over one's own. Putting the two together needs
// the thing itself, so this is where it is read.
package access

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/access"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Guard struct {
	runner runnerManager.Client
	guard  *access.Guard
}

func NewGuard(runner runnerManager.Client, guard *access.Guard) *Guard {
	return &Guard{runner: runner, guard: guard}
}

// OverContainer reports domain.ErrForbidden when this container is not one
// this person may act on. A container that is not there is not forbidden but
// missing, which the caller already knows how to say.
func (g *Guard) OverContainer(ctx context.Context, actorUUID string, all string, own string, containerUUID string) error {
	c, err := g.runner.Container(ctx, containerUUID)
	if err != nil {
		return err
	}

	return g.may(ctx, actorUUID, all, own, c.OwnerUUID)
}

// OverStack is the same question about a stack. Its services belong to whoever
// asked for the stack, so the stack alone answers it.
func (g *Guard) OverStack(ctx context.Context, actorUUID string, all string, own string, stackUUID string) error {
	s, err := g.runner.Stack(ctx, stackUUID)
	if err != nil {
		return err
	}

	return g.may(ctx, actorUUID, all, own, s.OwnerUUID)
}

func (g *Guard) may(ctx context.Context, actorUUID string, all string, own string, ownerUUID string) error {
	allowed, err := g.guard.May(ctx, actorUUID, all, own, ownerUUID)
	if err != nil {
		return err
	}

	if !allowed {
		return domain.ErrForbidden
	}

	return nil
}
