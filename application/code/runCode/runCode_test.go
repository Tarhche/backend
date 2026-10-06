package runCode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	workloadMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func payloadOf(t *testing.T, request Request) []byte {
	t.Helper()

	payload, err := json.Marshal(request)
	require.NoError(t, err)

	return payload
}

// refusing is the workload refusing what it is asked with codes, as the
// control plane's client says it refused something.
type refusing struct {
	codes domain.ValidationErrors
}

func (r *refusing) Error() string {
	return "the workload refused the request"
}

func (r *refusing) Refused() domain.ValidationErrors {
	return r.codes
}

func TestRunCode_Handle(t *testing.T) {
	t.Parallel()

	// asked is the task a snippet is asked to be run in.
	asked := func(t *testing.T, request Request) workloadControlPlane.TaskRequest {
		t.Helper()

		var (
			workload workloadMock.MockClient
			replyer  messagingMock.RecordingReplyer
			run      workloadControlPlane.TaskRequest
		)

		workload.On("RunTask", mock.Anything, task.GuestOwnerUUID, mock.Anything).Once().
			Run(func(args mock.Arguments) { run = args.Get(2).(workloadControlPlane.TaskRequest) }).
			Return(taskKind.Task{}, nil)
		defer workload.AssertExpectations(t)

		require.NoError(t, NewRunCodeHandler(accepts(), &workload, &replyer, discard()).Handle(context.Background(), payloadOf(t, request)))
		assert.Empty(t, replyer.Replies(), "it is answered once it has run, from what its node says")

		return run
	}

	t.Run("a snippet is run as a job of the guest's, named after its request", func(t *testing.T) {
		t.Parallel()

		run := asked(t, Request{ID: "request-id", Code: "console.log('hello')", Runner: "nodejs-22.14"})

		assert.Equal(t, "request-id", run.Name)
		assert.Equal(t, task.KindJob, run.Spec.Kind)
		assert.Equal(t, "ghcr.io/tarhche/code-runner:nodejs-22.14-latest", run.Spec.Image)
		assert.Equal(t, []string{"--timeout", "30", "console.log('hello')"}, run.Spec.Command)
		assert.Equal(t, time.Minute, run.Spec.TTL, "twice its timeout, the backstop for code that ignores it")
		assert.Equal(t, taskKind.Limits{CPU: DefaultMaxCpu, Memory: DefaultMaxMemorySize, Disk: DefaultMaxDiskSize}, run.Spec.Limits)
		require.NotNil(t, run.Spec.MaxRetries)
		assert.Equal(t, 0, *run.Spec.MaxRetries, "a snippet that could not be run is not tried again")
		assert.False(t, run.Spec.Interactive)
		assert.Empty(t, run.Spec.Ports)
	})

	t.Run("a Go snippet is given what building it takes", func(t *testing.T) {
		t.Parallel()

		run := asked(t, Request{ID: "request-id", Code: "package main\nfunc main() {}", Runner: "go-1.24"})

		assert.Equal(t, "ghcr.io/tarhche/code-runner:go-1.24-latest", run.Spec.Image)
		assert.Equal(t, []string{"--timeout", "30", "package main\nfunc main() {}"}, run.Spec.Command)
		assert.Equal(t, taskKind.Limits{CPU: DefaultMaxCpu, Memory: GoMaxMemorySize, Disk: GoMaxDiskSize}, run.Spec.Limits)
	})

	t.Run("a live snippet is watched, serves its ports, and is given its countdown", func(t *testing.T) {
		t.Parallel()

		run := asked(t, Request{ID: "request-id", Code: "serve", Runner: "nodejs-22.14", Ports: []port.Port{3000}, Terminal: true})

		assert.True(t, run.Spec.Interactive)
		assert.Equal(t, []port.Port{3000}, run.Spec.Ports)
		assert.Equal(t, []string{"--timeout", "120", "serve"}, run.Spec.Command)
		assert.Equal(t, LiveTTL, run.Spec.TTL)
	})

	t.Run("one somebody only has a way into is watched as well", func(t *testing.T) {
		t.Parallel()

		run := asked(t, Request{ID: "request-id", Code: "sleep", Runner: "nodejs-22.14", Terminal: true})

		assert.True(t, run.Spec.Interactive)
		assert.Empty(t, run.Spec.Ports)
	})

	t.Run("one that is not valid is answered, and nothing is asked for", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient
			replyer  messagingMock.RecordingReplyer
		)

		refuses := &validator.MockValidator{}
		refuses.On("Validate", mock.Anything).Return(domain.ValidationErrors{"runner": "runner is not valid"})

		require.NoError(t, NewRunCodeHandler(refuses, &workload, &replyer, discard()).Handle(context.Background(), payloadOf(t, Request{ID: "request-id", Runner: "python"})))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, "request-id", replies[0].RequestID)
		assert.JSONEq(t, `{"errors":{"runner":"runner is not valid"}}`, string(replies[0].Payload))
		workload.AssertNotCalled(t, "RunTask", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("what the workload refuses is answered in words, rather than asked again", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient
			replyer  messagingMock.RecordingReplyer
		)

		workload.On("RunTask", mock.Anything, task.GuestOwnerUUID, mock.Anything).Once().
			Return(taskKind.Task{}, &refusing{codes: domain.ValidationErrors{"limits.memory": "memory_below_minimum"}})

		words := &validator.MockValidator{}
		words.On("Validate", mock.AnythingOfType("*runCode.Request")).Return(domain.ValidationErrors{})
		words.On("Validate", codes{"limits.memory": "memory_below_minimum"}).Return(domain.ValidationErrors{"limits.memory": "too little memory"})

		require.NoError(t, NewRunCodeHandler(words, &workload, &replyer, discard()).Handle(context.Background(), payloadOf(t, Request{ID: "request-id", Code: "x", Runner: "go-1.24"})))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.JSONEq(t, `{"errors":{"limits.memory":"too little memory"}}`, string(replies[0].Payload))
	})

	t.Run("a workload that cannot be reached is asked again", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient
			replyer  messagingMock.RecordingReplyer
		)

		unreachable := errors.New("connection refused")
		workload.On("RunTask", mock.Anything, task.GuestOwnerUUID, mock.Anything).Once().Return(taskKind.Task{}, unreachable)

		err := NewRunCodeHandler(accepts(), &workload, &replyer, discard()).Handle(context.Background(), payloadOf(t, Request{ID: "request-id", Code: "x", Runner: "go-1.24"}))
		assert.ErrorIs(t, err, unreachable)
		assert.Empty(t, replyer.Replies())
	})
}
