// Package schedule hands a task to the node that is to run it.
//
// It is one place because it is asked for from three: when a task is first
// created, when one has drifted from what was asked of it, and when an attempt
// at one failed and is worth another. What the node needs is the same each
// time, and so is what it must be told about which attempt this is.
package schedule

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
)

// Scheduler asks nodes for tasks.
type Scheduler struct {
	producer domain.Producer
}

func New(producer domain.Producer) *Scheduler {
	return &Scheduler{
		producer: producer,
	}
}

// On asks the named node for this task, as the attempt-th try at it.
func (s *Scheduler) On(ctx context.Context, t *task.Task, nodeName string, attempt int) error {
	payload, err := json.Marshal(events.NewTaskScheduled(t, nodeName, attempt))
	if err != nil {
		return err
	}

	return s.producer.Produce(ctx, events.TaskScheduledName, payload)
}
