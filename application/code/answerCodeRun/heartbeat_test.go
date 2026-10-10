package answerCodeRun

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

const ingressDomain = "workload.example.com"

var started = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// snippet is a run of a job the code runner asked for, as its node reports
// it, in state, having written output.
func snippet(state kind.State, output string, change ...func(s *taskKind.Status)) taskKind.Status {
	s := taskKind.Status{
		Status: kind.Status{State: state},
		Run: &taskKind.Run{
			ID:        "execution-1",
			Name:      "request-id",
			Slug:      "request-id-abcde",
			Kind:      task.KindJob,
			StartedAt: started,
			Deadline:  started.Add(time.Minute),
			Output:    output,
		},
	}

	for _, c := range change {
		c(&s)
	}

	return s
}

func watched(s *taskKind.Status) {
	s.Run.Interactive = true
	s.Run.Deadline = started.Add(2 * time.Minute)
	s.Run.Endpoints = []taskKind.Endpoint{{Port: 3000, Address: "vmhost:20000"}, {Port: 8080, Address: "vmhost:20001"}}
}

// beat is a heartbeat of the task kind's from a node, saying what one of its
// tasks, by uuid, is doing.
func beat(t *testing.T, uuid string, status taskKind.Status) []byte {
	t.Helper()

	encoded, err := json.Marshal(status)
	require.NoError(t, err)

	payload, err := json.Marshal(kind.Heartbeat{
		Node:     "workload-orchestrator-01",
		At:       started,
		Observed: kind.Observation{Kind: taskKind.Name, UUID: uuid, Status: encoded},
	})
	require.NoError(t, err)

	return payload
}

// heard is what a node's tasks are answered with, by uuid, a heartbeat of
// each heard in turn.
func heard(t *testing.T, tasks map[string]taskKind.Status) []domain.Reply {
	t.Helper()

	var replyer messagingMock.RecordingReplyer

	handler := NewHeartbeatHandler(&replyer, ingressDomain, slog.New(slog.DiscardHandler))

	for _, uuid := range slices.Sorted(maps.Keys(tasks)) {
		require.NoError(t, handler.Handle(context.Background(), beat(t, uuid, tasks[uuid])))
	}

	return replyer.Replies()
}

func answer(t *testing.T, reply domain.Reply) map[string]any {
	t.Helper()

	var fields map[string]any
	require.NoError(t, json.Unmarshal(reply.Payload, &fields))

	return fields
}

func TestHeartbeat_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a snippet nobody is watching is answered once it ends, with what it printed", func(t *testing.T) {
		t.Parallel()

		replies := heard(t, map[string]taskKind.Status{"task-uuid": snippet(taskKind.Completed, "hello from e2e\n")})
		require.Len(t, replies, 1)

		assert.Equal(t, "request-id", replies[0].RequestID)
		assert.Equal(t, domain.ReplyFinal, replies[0].Kind)

		var response Response
		require.NoError(t, json.Unmarshal(replies[0].Payload, &response))
		assert.Equal(t, Response{TaskUUID: "task-uuid", Name: "request-id", Logs: []byte("hello from e2e\n"), State: "completed"}, response)
	})

	t.Run("and what its program exited with says whether it failed", func(t *testing.T) {
		t.Parallel()

		replies := heard(t, map[string]taskKind.Status{"task-uuid": snippet(taskKind.Failed, "still going\n⏰ Execution timed out after 30 seconds\n", func(s *taskKind.Status) {
			s.Reason = "the task failed"
			s.Run.ExitCode = 124
		})})
		require.Len(t, replies, 1)

		var response Response
		require.NoError(t, json.Unmarshal(replies[0].Payload, &response))
		assert.Equal(t, "failed", response.State)
		assert.Equal(t, "still going\n⏰ Execution timed out after 30 seconds\n", string(response.Logs))
		assert.Empty(t, response.Error, "a snippet that ran speaks through its own output")
	})

	t.Run("one still running is not answered yet", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, heard(t, map[string]taskKind.Status{"task-uuid": snippet(taskKind.Running, "hel")}))
	})

	t.Run("one that printed nothing says so as nothing", func(t *testing.T) {
		t.Parallel()

		replies := heard(t, map[string]taskKind.Status{"task-uuid": snippet(taskKind.Completed, "")})
		require.Len(t, replies, 1)
		assert.Nil(t, answer(t, replies[0])["logs"])
	})

	t.Run("a snippet somebody is watching is told what it does as it does it, and where it is reached", func(t *testing.T) {
		t.Parallel()

		replies := heard(t, map[string]taskKind.Status{"task-uuid": snippet(taskKind.Running, "listening\n", watched)})
		require.Len(t, replies, 1)
		assert.Equal(t, domain.ReplyChunk, replies[0].Kind)

		var response Response
		require.NoError(t, json.Unmarshal(replies[0].Payload, &response))

		deadline := started.Add(2 * time.Minute)
		assert.Equal(t, Response{
			TaskUUID: "task-uuid",
			Name:     "request-id",
			Logs:     []byte("listening\n"),
			State:    "running",
			Endpoints: []Endpoint{
				{TaskPort: 3000, URL: "http://request-id-abcde-3000." + ingressDomain},
				{TaskPort: 8080, URL: "http://request-id-abcde-8080." + ingressDomain},
			},
			Deadline: &deadline,
		}, response)
	})

	t.Run("and its last answer says it ended, with nothing left to reach", func(t *testing.T) {
		t.Parallel()

		replies := heard(t, map[string]taskKind.Status{"task-uuid": snippet(taskKind.Completed, "bye\n", watched)})
		require.Len(t, replies, 1)
		assert.Equal(t, domain.ReplyEOF, replies[0].Kind)

		fields := answer(t, replies[0])
		assert.Equal(t, "completed", fields["state"])
		assert.NotContains(t, fields, "endpoints")
		assert.NotContains(t, fields, "deadline")
	})

	t.Run("a service is somebody's task, not a request anybody made", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, heard(t, map[string]taskKind.Status{"task-uuid": snippet(taskKind.Stopped, "", func(s *taskKind.Status) { s.Run.Kind = task.KindService })}))
	})

	t.Run("nor is one with no run, no name, or nothing to say yet", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, heard(t, map[string]taskKind.Status{
			"no-run":    {Status: kind.Status{State: taskKind.Completed}},
			"no-name":   snippet(taskKind.Completed, "", func(s *taskKind.Status) { s.Run.Name = "" }),
			"no-saying": snippet("", "", watched),
		}))
	})

	t.Run("every snippet a heartbeat speaks of is answered, one in each", func(t *testing.T) {
		t.Parallel()

		replies := heard(t, map[string]taskKind.Status{
			"task-1": snippet(taskKind.Completed, "one\n", func(s *taskKind.Status) { s.Run.Name = "request-1" }),
			"task-2": snippet(taskKind.Completed, "two\n", func(s *taskKind.Status) { s.Run.Name = "request-2" }),
		})

		requests := make([]string, len(replies))
		for i := range replies {
			requests[i] = replies[i].RequestID
		}

		assert.ElementsMatch(t, []string{"request-1", "request-2"}, requests)
	})

	t.Run("a beat that cannot be read is let go of, so is a task whose status cannot be, and one of another kind speaks of no task, whatever it holds", func(t *testing.T) {
		t.Parallel()

		var replyer messagingMock.RecordingReplyer
		handler := NewHeartbeatHandler(&replyer, ingressDomain, slog.New(slog.DiscardHandler))

		assert.NoError(t, handler.Handle(context.Background(), []byte("{")))

		unreadable, err := json.Marshal(kind.Heartbeat{
			Node:     "workload-orchestrator-01",
			Observed: kind.Observation{Kind: taskKind.Name, UUID: "task-uuid", Status: json.RawMessage(`"running"`)},
		})
		require.NoError(t, err)
		assert.NoError(t, handler.Handle(context.Background(), unreadable))

		ended, err := json.Marshal(snippet(taskKind.Completed, "bye\n"))
		require.NoError(t, err)

		payload, err := json.Marshal(kind.Heartbeat{
			Node:     "workload-orchestrator-01",
			Observed: kind.Observation{Kind: "vm", UUID: "vm-1", Status: ended},
		})
		require.NoError(t, err)
		assert.NoError(t, handler.Handle(context.Background(), payload))

		assert.Empty(t, replyer.Replies())
	})
}

func TestDeadline(t *testing.T) {
	t.Parallel()

	ends := started.Add(90 * time.Second)

	t.Run("a snippet somebody is watching says when it will be stopped", func(t *testing.T) {
		t.Parallel()

		at := deadline(&taskKind.Run{Interactive: true, Deadline: ends}, taskKind.Running)

		require.NotNil(t, at)
		assert.Equal(t, ends, *at)
	})

	t.Run("a snippet nobody is watching is answered once and counts down to nothing", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, deadline(&taskKind.Run{Deadline: ends}, taskKind.Running))
	})

	t.Run("a snippet that is no longer running has nothing left", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, deadline(&taskKind.Run{Interactive: true, Deadline: ends}, taskKind.Completed))
	})

	t.Run("a task that may run for as long as it likes has no deadline", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, deadline(&taskKind.Run{Interactive: true}, taskKind.Running))
	})
}
