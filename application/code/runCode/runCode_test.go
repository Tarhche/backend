package runCode

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestRunCode_Handle(t *testing.T) {
	t.Parallel()

	// requested is the task a snippet is asked to be run in.
	requested := func(t *testing.T, request Request) events.TaskRunRequested {
		t.Helper()

		accepts := &validator.MockValidator{}
		accepts.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

		var (
			producer messagingMock.Recorder
			replyer  messagingMock.RecordingReplyer
		)

		payload, err := json.Marshal(request)
		require.NoError(t, err)

		handler := NewRunCodeHandler(accepts, &producer, &replyer, slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.NoError(t, handler.Handle(context.Background(), payload))

		var event events.TaskRunRequested
		require.True(t, producer.Last(events.TaskRunRequestedName, &event), "a task is asked for")

		return event
	}

	t.Run("a Go snippet is given what building it takes", func(t *testing.T) {
		t.Parallel()

		event := requested(t, Request{ID: "request-id", Code: "package main\nfunc main() {}", Runner: "go-1.24"})

		assert.Equal(t, "ghcr.io/tarhche/code-runner:go-1.24-latest", event.Image)
		assert.Equal(t, []string{"--timeout", "30", "package main\nfunc main() {}"}, event.Command)
		assert.Equal(t, events.ResourceLimits{Cpu: DefaultMaxCpu, Memory: GoMaxMemorySize, Disk: GoMaxDiskSize}, event.ResourceLimits)
	})

	t.Run("any other is given the defaults", func(t *testing.T) {
		t.Parallel()

		event := requested(t, Request{ID: "request-id", Code: "console.log('hello')", Runner: "nodejs-22.14"})

		assert.Equal(t, "ghcr.io/tarhche/code-runner:nodejs-22.14-latest", event.Image)
		assert.Equal(t, events.ResourceLimits{Cpu: DefaultMaxCpu, Memory: DefaultMaxMemorySize, Disk: DefaultMaxDiskSize}, event.ResourceLimits)
	})
}
