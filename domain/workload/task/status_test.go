package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateTaskState(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name     string
		status   Status
		kind     Kind
		exitCode int
		want     State
	}{
		{
			name:   "a job that exits has finished",
			status: StatusExited,
			kind:   KindJob,
			want:   Completed,
		},
		{
			// what it returned is the difference between a job that ran to the
			// end and one that fell over, and a reader watching a snippet is
			// owed the second answer rather than the first.
			name:     "a job that returns a failure has not completed",
			status:   StatusExited,
			kind:     KindJob,
			exitCode: 3,
			want:     Failed,
		},
		{
			// a task stopped, killed, or taken away when its time was up
			// is ended from outside: docker reports that as 128 plus the
			// signal, and none of it is the code's doing.
			name:     "a job ended by a signal did not fail",
			status:   StatusExited,
			kind:     KindJob,
			exitCode: 137,
			want:     Completed,
		},
		{
			// whether a service that ended was supposed to is not something a
			// task can say, so what it returned changes nothing here.
			name:     "a service that returns a failure has still stopped",
			status:   StatusExited,
			kind:     KindService,
			exitCode: 3,
			want:     Stopped,
		},
		{
			// the difference that matters: a service is meant to keep going, so
			// its exit is a run that ended rather than one that completed — and
			// a stopped service can be started again, which a completed job
			// cannot.
			name:   "a service that exits has stopped",
			status: StatusExited,
			kind:   KindService,
			want:   Stopped,
		},
		{
			name:   "a task being removed is read the same way",
			status: StatusRemoving,
			kind:   KindService,
			want:   Stopped,
		},
		{
			name:   "a running task is running, whatever it is",
			status: StatusRunning,
			kind:   KindService,
			want:   Running,
		},
		{
			name:   "a created task has been scheduled but not started",
			status: StatusCreated,
			kind:   KindService,
			want:   Scheduled,
		},
		{
			name:   "a restarting task is restarting",
			status: StatusRestarting,
			kind:   KindService,
			want:   Restarting,
		},
		{
			name:   "a paused task is stopped as far as anyone can tell",
			status: StatusPaused,
			kind:   KindJob,
			want:   Stopped,
		},
		{
			name:   "a dead task failed",
			status: StatusDead,
			kind:   KindService,
			want:   Failed,
		},
		{
			name:   "a status nobody recognises is not something to call healthy",
			status: Status(99),
			kind:   KindService,
			want:   Failed,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, EvaluateState(tt.status, tt.kind, tt.exitCode))
		})
	}
}

func TestStoppedServiceCanRun_Again(t *testing.T) {
	t.Parallel()

	// the two states a service reaches by being stopped or by falling over are
	// both states it can be brought back from. A task is immutable, but
	// that is about its specification, not about whether it may run again.
	for _, state := range []State{Stopped, Completed, Failed} {
		assert.True(t, ValidStateTransition(state, Restarting), "from %s", state)
	}
}
