// Package schedule hands a container to the node that is to run it.
//
// It is one place because it is asked for from three: when a container is first
// created, when one has drifted from what was asked of it, and when an attempt
// at one failed and is worth another. What the node needs is the same each
// time, and so is what it must be told about which attempt this is.
package schedule

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// Scheduler asks nodes for containers.
type Scheduler struct {
	stackRepository stack.Repository
	producer        domain.Producer
}

func New(stackRepository stack.Repository, producer domain.Producer) *Scheduler {
	return &Scheduler{
		stackRepository: stackRepository,
		producer:        producer,
	}
}

// On asks the named node for this container, as the attempt-th try at it.
//
// The node is a nomination rather than a decision: a service of a stack goes
// where its stack is, whoever nominated what. Everything in a stack shares one
// private network, and a bridge is local to the node that created it, so a
// stack runs on one node or it does not run.
func (s *Scheduler) On(ctx context.Context, t *task.Task, nodeName string, attempt int) error {
	holder, err := s.stackOf(ctx, t)
	if err != nil {
		return err
	}

	if len(holder.NodeName) > 0 {
		nodeName = holder.NodeName
	}

	payload, err := json.Marshal(events.NewTaskScheduled(t, holder.Slug, nodeName, attempt))
	if err != nil {
		return err
	}

	return s.producer.Produce(ctx, events.TaskScheduledName, payload)
}

// stackOf is the stack a container is a service of, if it is one: where it
// runs, and what its network is called there — the slug, not the uuid, is what
// names the docker network its services share.
func (s *Scheduler) stackOf(ctx context.Context, t *task.Task) (stack.Stack, error) {
	if len(t.StackUUID) == 0 {
		return stack.Stack{}, nil
	}

	holder, err := s.stackRepository.GetOne(ctx, t.StackUUID)
	if errors.Is(err, domain.ErrNotExists) {
		// a service whose stack is gone is on its own, and where it is is
		// where it stays.
		return stack.Stack{}, nil
	}

	return holder, err
}
