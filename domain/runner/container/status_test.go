package container

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

func TestEvaluateTaskState(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name     string
		status   Status
		kind     task.Kind
		exitCode int
		want     task.State
	}{
		{
			name:   "a job that exits has finished",
			status: StatusExited,
			kind:   task.KindJob,
			want:   task.Completed,
		},
		{
			// what it returned is the difference between a job that ran to the
			// end and one that fell over, and a reader watching a snippet is
			// owed the second answer rather than the first.
			name:     "a job that returns a failure has not completed",
			status:   StatusExited,
			kind:     task.KindJob,
			exitCode: 3,
			want:     task.Failed,
		},
		{
			// a container stopped, killed, or taken away when its time was up
			// is ended from outside: docker reports that as 128 plus the
			// signal, and none of it is the code's doing.
			name:     "a job ended by a signal did not fail",
			status:   StatusExited,
			kind:     task.KindJob,
			exitCode: 137,
			want:     task.Completed,
		},
		{
			// whether a service that ended was supposed to is not something a
			// container can say, so what it returned changes nothing here.
			name:     "a service that returns a failure has still stopped",
			status:   StatusExited,
			kind:     task.KindService,
			exitCode: 3,
			want:     task.Stopped,
		},
		{
			// the difference that matters: a service is meant to keep going, so
			// its exit is a run that ended rather than one that completed — and
			// a stopped service can be started again, which a completed job
			// cannot.
			name:   "a service that exits has stopped",
			status: StatusExited,
			kind:   task.KindService,
			want:   task.Stopped,
		},
		{
			name:   "a container being removed is read the same way",
			status: StatusRemoving,
			kind:   task.KindService,
			want:   task.Stopped,
		},
		{
			name:   "a running container is running, whatever it is",
			status: StatusRunning,
			kind:   task.KindService,
			want:   task.Running,
		},
		{
			name:   "a created container has been scheduled but not started",
			status: StatusCreated,
			kind:   task.KindService,
			want:   task.Scheduled,
		},
		{
			name:   "a restarting container is restarting",
			status: StatusRestarting,
			kind:   task.KindService,
			want:   task.Restarting,
		},
		{
			name:   "a paused container is stopped as far as anyone can tell",
			status: StatusPaused,
			kind:   task.KindJob,
			want:   task.Stopped,
		},
		{
			name:   "a dead container failed",
			status: StatusDead,
			kind:   task.KindService,
			want:   task.Failed,
		},
		{
			name:   "a status nobody recognises is not something to call healthy",
			status: Status(99),
			kind:   task.KindService,
			want:   task.Failed,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, EvaluateTaskState(tt.status, tt.kind, tt.exitCode))
		})
	}
}

func TestStoppedServiceCanRun_Again(t *testing.T) {
	t.Parallel()

	// the two states a service reaches by being stopped or by falling over are
	// both states it can be brought back from. A container is immutable, but
	// that is about its specification, not about whether it may run again.
	for _, state := range []task.State{task.Stopped, task.Completed, task.Failed} {
		assert.True(t, task.ValidStateTransition(state, task.Restarting), "from %s", state)
	}
}
