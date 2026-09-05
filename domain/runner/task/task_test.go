package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTask_Removable(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		task Task
		want bool
	}{
		"a job that has finished is not kept": {
			task: Task{Kind: KindJob, CurrentState: Completed},
			want: true,
		},
		"a job that failed is not kept either": {
			task: Task{Kind: KindJob, CurrentState: Failed},
			want: true,
		},
		"a job still running is kept": {
			task: Task{Kind: KindJob, CurrentState: Running},
			want: false,
		},
		"a stopped service is kept, because it can be started again": {
			task: Task{Kind: KindService, CurrentState: Stopped},
			want: false,
		},
		"anything that asked to be removed still is": {
			task: Task{Kind: KindService, CurrentState: Stopped, AutoRemove: true},
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.task.Removable())
		})
	}
}

func TestTask_OutlivedTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	for name, tt := range map[string]struct {
		task Task
		want bool
	}{
		"a job past what it asked for": {
			task: Task{Kind: KindJob, StartedAt: now.Add(-time.Minute), TTL: 30 * time.Second},
			want: true,
		},
		"a job still inside it": {
			task: Task{Kind: KindJob, StartedAt: now.Add(-time.Second), TTL: 30 * time.Second},
			want: false,
		},
		"a job that asked for no limit": {
			task: Task{Kind: KindJob, StartedAt: now.Add(-time.Hour)},
			want: false,
		},
		"a job that has not started yet, however long it has waited for a node": {
			task: Task{Kind: KindJob, TTL: time.Second},
			want: false,
		},
		"a service, which is meant to keep running": {
			task: Task{Kind: KindService, StartedAt: now.Add(-time.Hour), TTL: time.Second},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.task.OutlivedTTL(now))
		})
	}
}

func TestTask_Drifted(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	const silence = 15 * time.Second

	fresh := now.Add(-time.Second)
	quiet := now.Add(-time.Minute)

	for name, tt := range map[string]struct {
		task Task
		want bool
	}{
		"running as asked": {
			task: Task{ExpectedState: Running, CurrentState: Running, LastHeartbeatAt: fresh},
			want: false,
		},
		"stopped when it was meant to run": {
			task: Task{ExpectedState: Running, CurrentState: Stopped, LastHeartbeatAt: fresh},
			want: true,
		},
		"running when it was meant to stop": {
			task: Task{ExpectedState: Stopped, CurrentState: Running, LastHeartbeatAt: fresh},
			want: true,
		},
		"on its way to stopping": {
			task: Task{ExpectedState: Stopped, CurrentState: Stopping, LastHeartbeatAt: fresh},
			want: false,
		},
		"on its way, but nobody has spoken for it in a while": {
			task: Task{ExpectedState: Stopped, CurrentState: Stopping, LastHeartbeatAt: quiet},
			want: true,
		},
		"last seen running, but gone quiet: it is not there any more": {
			task: Task{ExpectedState: Running, CurrentState: Running, LastHeartbeatAt: quiet},
			want: true,
		},
		"nothing was ever asked of it": {
			task: Task{CurrentState: Running, LastHeartbeatAt: quiet},
			want: false,
		},
		"never heard from at all, and still on its way": {
			task: Task{ExpectedState: Running, CurrentState: Scheduled},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.task.Drifted(now, silence))
		})
	}
}
