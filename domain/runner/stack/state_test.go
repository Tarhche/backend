package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

func services(states ...task.State) []task.Task {
	items := make([]task.Task, len(states))
	for i, state := range states {
		items[i] = task.Task{CurrentState: state}
	}

	return items
}

func TestState(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name     string
		services []task.Task
		want     task.State
	}{
		{
			name:     "a stack whose services are all up is running",
			services: services(task.Running, task.Running, task.Running),
			want:     task.Running,
		},
		{
			name:     "a stack with something still starting is not running yet",
			services: services(task.Running, task.Scheduled),
			want:     task.Scheduled,
		},
		{
			name:     "a stack still being created is on its way",
			services: services(task.Created, task.Created),
			want:     task.Scheduled,
		},
		{
			name:     "a stack whose services have all stopped is stopped",
			services: services(task.Stopped, task.Stopped),
			want:     task.Stopped,
		},
		{
			name:     "a stack of finished jobs is stopped",
			services: services(task.Completed, task.Stopped),
			want:     task.Stopped,
		},
		{
			name:     "a stack that lost a service is failed, even with the rest up",
			services: services(task.Running, task.Failed),
			want:     task.Failed,
		},
		{
			name:     "a stack whose services all failed is failed",
			services: services(task.Failed, task.Failed),
			want:     task.Failed,
		},
		{
			name:     "a stack still stopping is not stopped yet",
			services: services(task.Stopping, task.Stopped),
			want:     task.Scheduled,
		},
		{
			name:     "a stack with no services at all is not a working stack",
			services: nil,
			want:     task.Failed,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, State(tt.services))
		})
	}
}

func TestExpectedState(t *testing.T) {
	t.Parallel()

	t.Run("a stack asked to stop is a stack asked to stop", func(t *testing.T) {
		t.Parallel()

		// its services are still stopping, which reads as a stack on its way
		// somewhere: where it is going is what this says.
		services := []task.Task{
			{CurrentState: task.Stopping, ExpectedState: task.Stopped},
			{CurrentState: task.Stopped, ExpectedState: task.Stopped},
		}

		assert.Equal(t, task.Stopped, ExpectedState(services))
	})

	t.Run("one service still wanted running keeps the stack wanted running", func(t *testing.T) {
		t.Parallel()

		services := []task.Task{
			{ExpectedState: task.Stopped},
			{ExpectedState: task.Running},
		}

		assert.Equal(t, task.Running, ExpectedState(services))
	})

	t.Run("a stack from before there were expectations was asked for nothing", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, task.State(0), ExpectedState([]task.Task{{}}))
	})
}
