package logs

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

const (
	taskUUID  = "task-uuid"
	requestID = "server-side-request-id"
)

var written = time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func logged(t *testing.T, uuid string, lines ...taskEvents.LogLine) []byte {
	t.Helper()

	payload, err := json.Marshal(taskEvents.TaskLogged{UUID: uuid, Lines: lines})
	require.NoError(t, err)

	return payload
}

func line(content string, at time.Time) taskEvents.LogLine {
	return taskEvents.LogLine{Stream: uint8(task.StreamStdout), Content: content, At: at}
}

// sent is what each reply says, in order.
func sent(t *testing.T, replyer *messagingMock.RecordingReplyer) []Response {
	t.Helper()

	chunks := replyer.Chunks()
	lines := make([]Response, len(chunks))

	for i, chunk := range chunks {
		require.NoError(t, json.Unmarshal(chunk, &lines[i]))
	}

	return lines
}

func contents(lines []Response) []string {
	said := make([]string, len(lines))
	for i, l := range lines {
		said[i] = l.Content
	}

	return said
}

func TestFollowers(t *testing.T) {
	t.Parallel()

	t.Run("a follower is sent what the task writes", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		followers := NewFollowers(&replyer, discardLogger())

		follower := followers.Follow(requestID, taskUUID, time.Time{})
		follower.CatchUp(context.Background(), nil)

		require.NoError(t, followers.Lines(context.Background(), logged(t, taskUUID,
			line("listening on :80", written),
			line("a request", written.Add(time.Second)),
		)))

		assert.Equal(t, []string{"listening on :80", "a request"}, contents(sent(t, &replyer)))
		assert.Equal(t, "stdout", sent(t, &replyer)[0].Stream)
	})

	t.Run("what another task writes is not this follower's news", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		followers := NewFollowers(&replyer, discardLogger())
		followers.Follow(requestID, taskUUID, time.Time{}).CatchUp(context.Background(), nil)

		require.NoError(t, followers.Lines(context.Background(), logged(t, "another-task", line("not mine", written))))

		assert.Empty(t, replyer.Replies())
	})

	t.Run("the backlog comes first, and what arrived meanwhile after it", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		followers := NewFollowers(&replyer, discardLogger())

		follower := followers.Follow(requestID, taskUUID, time.Time{})

		// the task writes while what it wrote before is still being read.
		require.NoError(t, followers.Lines(context.Background(), logged(t, taskUUID, line("third", written.Add(2*time.Second)))))

		assert.Empty(t, replyer.Replies(), "nothing is sent before the backlog it belongs after")

		follower.CatchUp(context.Background(), []task.Log{
			{TaskUUID: taskUUID, LogLine: task.LogLine{Content: "first", At: written}},
			{TaskUUID: taskUUID, LogLine: task.LogLine{Content: "second", At: written.Add(time.Second)}},
		})

		assert.Equal(t, []string{"first", "second", "third"}, contents(sent(t, &replyer)))
	})

	t.Run("a line the follower has already seen is not sent again", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		followers := NewFollowers(&replyer, discardLogger())

		// the page was rendered with everything up to here, so this is where
		// the follower starts.
		follower := followers.Follow(requestID, taskUUID, written)
		follower.CatchUp(context.Background(), []task.Log{
			{LogLine: task.LogLine{Content: "already read", At: written}},
			{LogLine: task.LogLine{Content: "new", At: written.Add(time.Second)}},
		})

		// the batch the store was written from arrives too, carrying both.
		require.NoError(t, followers.Lines(context.Background(), logged(t, taskUUID,
			line("already read", written),
			line("new", written.Add(time.Second)),
			line("newer", written.Add(2*time.Second)),
		)))

		assert.Equal(t, []string{"new", "newer"}, contents(sent(t, &replyer)))
	})

	t.Run("a task that is gone ends the follows of it", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		followers := NewFollowers(&replyer, discardLogger())
		followers.Follow(requestID, taskUUID, time.Time{}).CatchUp(context.Background(), nil)

		deleted, err := json.Marshal(taskEvents.TaskDeleted{UUID: taskUUID})
		require.NoError(t, err)
		require.NoError(t, followers.Deleted(context.Background(), deleted))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, domain.ReplyEOF, replies[0].Kind)
		assert.Zero(t, followers.Len())
	})

	t.Run("a client that walks away is no longer followed", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		followers := NewFollowers(&replyer, discardLogger())
		followers.Follow(requestID, taskUUID, time.Time{}).CatchUp(context.Background(), nil)

		followers.Remove(requestID)

		require.NoError(t, followers.Lines(context.Background(), logged(t, taskUUID, line("nobody is reading this", written))))

		assert.Empty(t, replyer.Replies())
		assert.Zero(t, followers.Len())
	})

	t.Run("a malformed report is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		followers := NewFollowers(&replyer, discardLogger())
		followers.Follow(requestID, taskUUID, time.Time{}).CatchUp(context.Background(), nil)

		assert.NoError(t, followers.Lines(context.Background(), []byte("{")))
		assert.Empty(t, replyer.Replies())
	})
}
