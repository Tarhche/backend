package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

// failedCreate is what a node says of a snippet's task it could not run.
func failedCreate(t *testing.T, change ...func(r *kind.Result, s *taskKind.Status)) []byte {
	t.Helper()

	status := taskKind.Status{
		Status: kind.Status{State: taskKind.Failed, Reason: "no such image: ghcr.io/example/workload:latest"},
		Run:    &taskKind.Run{Name: "request-id", Slug: "request-id-abcde", Kind: task.KindJob},
	}

	result := kind.Result{ID: "command-1", Kind: taskKind.Name, UUID: "task-uuid", Action: taskKind.ActionCreate, Node: "workload-orchestrator-01", Reason: "no such image: ghcr.io/example/workload:latest"}

	for _, c := range change {
		c(&result, &status)
	}

	encoded, err := json.Marshal(status)
	require.NoError(t, err)

	result.Status = encoded

	payload, err := json.Marshal(result)
	require.NoError(t, err)

	return payload
}

func TestResult_Handle(t *testing.T) {
	t.Parallel()

	t.Run("somebody whose code never ran is told why", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		require.NoError(t, NewResultHandler(&replyer, slog.New(slog.DiscardHandler)).Handle(context.Background(), failedCreate(t)))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, "request-id", replies[0].RequestID)

		var response Response
		require.NoError(t, json.Unmarshal(replies[0].Payload, &response))

		assert.Equal(t, "request-id", response.Name)
		assert.Contains(t, response.Error, "no such image")
		assert.Empty(t, response.Logs, "there is no output from something that never ran")
		assert.JSONEq(t, `{"name":"request-id","logs":null,"error":"no such image: ghcr.io/example/workload:latest"}`, string(replies[0].Payload))
	})

	for name, change := range map[string]func(r *kind.Result, s *taskKind.Status){
		"a create carried out says nothing here":  func(r *kind.Result, _ *taskKind.Status) { r.OK, r.Reason = true, "" },
		"nor does another of a task's commands":   func(r *kind.Result, _ *taskKind.Status) { r.Action = taskKind.ActionStop },
		"nor what came of another kind's":         func(r *kind.Result, _ *taskKind.Status) { r.Kind = "vm" },
		"nor a failure that says nothing of why":  func(r *kind.Result, _ *taskKind.Status) { r.Reason = "" },
		"nor one of a task that is not a request": func(_ *kind.Result, s *taskKind.Status) { s.Run.Kind = task.KindService },
		"nor one whose run says of no request":    func(_ *kind.Result, s *taskKind.Status) { s.Run = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var replyer messagingMock.RecordingReplyer

			require.NoError(t, NewResultHandler(&replyer, slog.New(slog.DiscardHandler)).Handle(context.Background(), failedCreate(t, change)))
			assert.Empty(t, replyer.Replies())
		})
	}

	t.Run("a result that cannot be read is let go of", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer

		assert.NoError(t, NewResultHandler(&replyer, slog.New(slog.DiscardHandler)).Handle(context.Background(), []byte("{")))
		assert.Empty(t, replyer.Replies())
	})
}
