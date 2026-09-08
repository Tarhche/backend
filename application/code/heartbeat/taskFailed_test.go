package heartbeat

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func failure(t *testing.T, failed events.TaskFailed) []byte {
	t.Helper()

	payload, err := json.Marshal(failed)
	require.NoError(t, err)

	return payload
}

func TestTaskFailed_Handle(t *testing.T) {
	t.Parallel()

	t.Run("somebody whose code never ran is told why", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		require.NoError(t, NewTaskFailedHandler(&replyer, discardLogger()).Handle(context.Background(), failure(t, events.TaskFailed{
			UUID:     "task-uuid",
			Name:     "a-request-id",
			NodeName: "runner-worker-01",
			At:       time.Now(),
			Reason:   "no such image: ghcr.io/example/runner:latest",
		})))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, "a-request-id", replies[0].RequestID)

		var answer Response
		require.NoError(t, json.Unmarshal(replies[0].Payload, &answer))

		assert.Equal(t, "a-request-id", answer.Name)
		assert.Contains(t, answer.Error, "no such image")
		assert.Empty(t, answer.Logs, "there is no output from something that never ran")
	})

	t.Run("a container that ran and failed speaks through its own output", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		// a failure with no reason is a container that ran: its heartbeat
		// carries the log, and answering here would answer twice.
		require.NoError(t, NewTaskFailedHandler(&replyer, discardLogger()).Handle(context.Background(), failure(t, events.TaskFailed{
			UUID: "task-uuid",
			Name: "a-request-id",
			At:   time.Now(),
		})))

		assert.Empty(t, replyer.Replies())
	})
}
