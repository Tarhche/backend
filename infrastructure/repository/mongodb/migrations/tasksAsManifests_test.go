package migrations

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

func TestTaskStates(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		state, expected int
		want, wanted    string
	}{
		"one being placed is to be run":                   {state: 1, expected: 3, want: "created", wanted: "running"},
		"and so is one being run, which its node takes":   {state: 2, expected: 3, want: "created", wanted: "running"},
		"a running one runs":                              {state: 3, expected: 3, want: "running", wanted: "running"},
		"one being stopped runs, and is to be stopped":    {state: 4, expected: 5, want: "running", wanted: "stopped"},
		"a stopped one stays stopped":                     {state: 5, expected: 5, want: "stopped", wanted: "stopped"},
		"a job that completed completed, as it was to":    {state: 6, expected: 6, want: "completed", wanted: "completed"},
		"one that failed failed":                          {state: 7, expected: 7, want: "failed", wanted: "failed"},
		"one restarting runs":                             {state: 8, expected: 3, want: "running", wanted: "running"},
		"one that says nothing it was asked is to be run": {state: 3, expected: 0, want: "running", wanted: "running"},
		"one in a state nobody knows failed":              {state: 42, expected: 3, want: "failed", wanted: "running"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state, expected := taskStates(tt.state, tt.expected)

			assert.Equal(t, tt.want, state)
			assert.Equal(t, tt.wanted, expected)
		})
	}
}

// running is a snippet the code runner was running when the tasks were
// converted, as it was kept.
func running() oldTask {
	created := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	none := int64(0)

	return oldTask{
		UUID:            "task-uuid",
		Name:            "0199b3c2-request",
		Slug:            "0199b3c2-request-abcde",
		Kind:            "job",
		CurrentState:    3,
		ExpectedState:   3,
		LastHeartbeatAt: created.Add(3 * time.Second),
		Image:           "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
		ExposedPorts:    []int64{8080, 3000},
		NetworkPolicy:   "isolated",
		Command:         []string{"--timeout", "120", "serve"},
		Interactive:     true,
		MaxRetries:      &none,
		TTL:             int64(2 * time.Minute),
		Deadline:        created.Add(time.Second + 2*time.Minute),
		ResourceLimits:  oldLimits{CPU: 2, Memory: 200 << 20, Disk: 100 << 20},
		NodeName:        "workload-orchestrator-01",
		ExecutionLogs:   []byte("listening\n"),
		ExecutionID:     "execution-id",
		OwnerUUID:       "guest",
		CreatedAt:       created,
		StartedAt:       created.Add(time.Second),
	}
}

func TestTaskManifest(t *testing.T) {
	t.Parallel()

	t.Run("a snippet in flight is the task kind's manifest of it", func(t *testing.T) {
		t.Parallel()

		encoded, err := bson.MarshalExtJSON(taskManifest(running()), false, false)
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"_id": "task-uuid",
			"kind": "task",
			"metadata": {
				"name": "0199b3c2-request",
				"slug": "0199b3c2-request-abcde",
				"owner_uuid": "guest",
				"node": "workload-orchestrator-01",
				"created_at": {"$date": "2026-10-06T12:00:00Z"},
				"updated_at": {"$date": "2026-10-06T12:00:01Z"}
			},
			"spec": {
				"kind": "job",
				"image": "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
				"command": ["--timeout", "120", "serve"],
				"ports": [3000, 8080],
				"network_policy": "isolated",
				"interactive": true,
				"ttl": 120000000000,
				"limits": {"cpu": 2.0, "memory": 209715200, "disk": 104857600},
				"max_retries": 0
			},
			"status": {
				"state": "running",
				"expected": "running",
				"since": "2026-10-06T12:00:01Z",
				"observed_at": "2026-10-06T12:00:03Z",
				"run": {
					"id": "execution-id",
					"name": "0199b3c2-request",
					"slug": "0199b3c2-request-abcde",
					"kind": "job",
					"interactive": true,
					"started_at": "2026-10-06T12:00:01Z",
					"deadline": "2026-10-06T12:02:01Z",
					"output": "listening\n"
				}
			},
			"version": 1,
			"control": {}
		}`, string(encoded))
	})

	t.Run("and reads as the kind's own", func(t *testing.T) {
		t.Parallel()

		converted := taskManifest(running())

		var spec taskKind.Spec
		var status taskKind.Status

		for _, part := range []struct {
			key  string
			into any
		}{{key: "spec", into: &spec}, {key: "status", into: &status}} {
			index := -1
			for i, e := range converted {
				if e.Key == part.key {
					index = i
				}
			}

			require.GreaterOrEqual(t, index, 0)

			relaxed, err := bson.MarshalExtJSON(converted[index].Value, false, false)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(relaxed, part.into))
		}

		assert.Equal(t, task.KindJob, spec.Kind)
		assert.Equal(t, []port.Port{3000, 8080}, spec.Ports)
		assert.Equal(t, network.PolicyIsolated, spec.NetworkPolicy)
		assert.Equal(t, 2*time.Minute, spec.TTL)
		assert.Equal(t, taskKind.Limits{CPU: 2, Memory: 200 << 20, Disk: 100 << 20}, spec.Limits)
		require.NotNil(t, spec.MaxRetries)
		assert.Equal(t, 0, *spec.MaxRetries)

		assert.Equal(t, kind.State("running"), status.State)
		require.NotNil(t, status.Run)
		assert.Equal(t, "0199b3c2-request", status.Run.Name)
		assert.Equal(t, time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC), status.Run.StartedAt)
		assert.Equal(t, "listening\n", status.Run.Output)
		assert.Empty(t, kind.Check(taskKind.Descriptor()), "what it is read as is the kind there is")
	})

	t.Run("one from before there were kinds and policies is a public job, worth no retries", func(t *testing.T) {
		t.Parallel()

		old := running()
		old.Kind, old.NetworkPolicy, old.MaxRetries = "", "", nil

		encoded, err := bson.MarshalExtJSON(taskManifest(old), false, false)
		require.NoError(t, err)

		assert.Contains(t, string(encoded), `"kind":"job"`)
		assert.Contains(t, string(encoded), `"network_policy":"public"`)
		assert.Contains(t, string(encoded), `"max_retries":0`)
	})

	t.Run("a service worth what a service is, and one that failed saying why", func(t *testing.T) {
		t.Parallel()

		old := running()
		old.Kind, old.MaxRetries, old.TTL = "service", nil, 0
		old.CurrentState, old.Reason = 7, ""

		encoded, err := bson.MarshalExtJSON(taskManifest(old), false, false)
		require.NoError(t, err)

		assert.Contains(t, string(encoded), `"max_retries":3`)
		assert.Contains(t, string(encoded), `"reason":"the task failed"`)
		assert.NotContains(t, string(encoded), `"ttl"`)
	})

	t.Run("one never run has no run, nor a slug it never had", func(t *testing.T) {
		t.Parallel()

		old := running()
		old.CurrentState, old.ExecutionID, old.StartedAt, old.ExecutionLogs, old.Slug, old.NodeName = 1, "", time.Time{}, nil, "", ""

		encoded, err := bson.MarshalExtJSON(taskManifest(old), false, false)
		require.NoError(t, err)

		assert.NotContains(t, string(encoded), `"run"`)
		assert.NotContains(t, string(encoded), `"slug"`)
		assert.NotContains(t, string(encoded), `"node"`)
		assert.Contains(t, string(encoded), `"state":"created"`)
	})

	t.Run("what it printed is kept to its end", func(t *testing.T) {
		t.Parallel()

		old := running()
		old.ExecutionLogs = []byte(strings.Repeat("a", maxTaskOutput) + "the end\n")

		run := taskRun(old, "job")

		var output string
		for _, e := range run {
			if e.Key == "output" {
				output = e.Value.(string)
			}
		}

		assert.Len(t, output, maxTaskOutput)
		assert.True(t, strings.HasSuffix(output, "the end\n"))
	})

	t.Run("its ports are those it exposed and those it was bound to, each once", func(t *testing.T) {
		t.Parallel()

		old := running()
		old.ExposedPorts = []int64{8080}
		old.PortBindings = []map[string]bson.RawValue{{"9090": {}}, {"8080": {}}}

		assert.Equal(t, bson.A{int64(8080), int64(9090)}, taskPorts(old))
	})
}
