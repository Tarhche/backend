package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

func services(states ...task.State) []task.Task {
	items := make([]task.Task, len(states))
	for i, state := range states {
		items[i] = task.Task{State: state}
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
