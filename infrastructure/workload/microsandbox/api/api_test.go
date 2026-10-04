package api_test

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// assertPinned holds a value to the JSON pinned for it in testdata, both ways:
// the value encodes to exactly that document, and the document decodes to
// exactly that value.
//
// The documents were written from the contract rather than generated from the
// code, so a test failing here is the wire changing. Within a major version
// that is never an accident: a field is only ever added, to the document as
// much as to the type, and nothing is renamed, retyped or dropped.
func assertPinned[T any](t *testing.T, file string, value T) {
	t.Helper()

	pinned, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err)

	encoded, err := json.Marshal(value)
	require.NoError(t, err)

	// both are laid out the same way before they are compared, so that a
	// difference shows line by line. Everything else has to match exactly:
	// every key, its order, whether it is there at all, and how its value is
	// written.
	assert.Equal(t, layout(t, pinned), layout(t, encoded), "%T does not encode to %s", value, file)

	var decoded T
	require.NoError(t, json.Unmarshal(pinned, &decoded))
	assert.Equal(t, value, decoded, "%s does not decode to the %T it pins", file, value)

	// the two sides are built from different commits, so whichever is older
	// has to read past what the newer one added.
	var object map[string]json.RawMessage
	if json.Unmarshal(pinned, &object) != nil {
		return
	}

	object["added_by_a_later_commit"] = json.RawMessage(`{"anything":[1,"two",null]}`)

	extended, err := json.Marshal(object)
	require.NoError(t, err)

	var tolerant T
	require.NoError(t, json.Unmarshal(extended, &tolerant))
	assert.Equal(t, value, tolerant, "%s with a field added does not decode to the %T it pins", file, value)
}

// layout lays a document out the one way both sides of a comparison share.
func layout(t *testing.T, document []byte) string {
	t.Helper()

	var compact, indented bytes.Buffer
	require.NoError(t, json.Compact(&compact, document))
	require.NoError(t, json.Indent(&indented, compact.Bytes(), "", "  "))

	return indented.String()
}

var (
	createdAt  = time.Date(2026, time.October, 4, 9, 30, 0, 0, time.UTC)
	startedAt  = time.Date(2026, time.October, 4, 9, 30, 1, 523847291, time.UTC)
	finishedAt = time.Date(2026, time.October, 4, 9, 41, 17, 250000000, time.UTC)
)

// serviceTask sets every field a task has.
func serviceTask() api.Task {
	return api.Task{
		UUID:        "4f1c2a9e-8d3b-4c7a-9e2f-1a2b3c4d5e6f",
		Name:        "web",
		Slug:        "web-xkfqz",
		Kind:        "service",
		Owner:       "8a7b6c5d-4e3f-4a1b-9c8d-7e6f5a4b3c2d",
		Stack:       "c3d2e1f0-a9b8-4c7d-8e6f-5a4b3c2d1e0f",
		Attempt:     2,
		Interactive: true,
		TTLSeconds:  3600,
	}
}

// jobTask sets only what a task always has.
func jobTask() api.Task {
	return api.Task{
		UUID: "9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b",
		Name: "snippet",
		Slug: "snippet-q7w2e",
		Kind: "job",
	}
}

// serviceSpec sets every field a spec has.
func serviceSpec() api.RunSpec {
	return api.RunSpec{
		Node:          "orchestrator-01",
		Name:          "web-xkfqz",
		Image:         "nginx:alpine",
		Entrypoint:    []string{"/docker-entrypoint.sh"},
		Command:       []string{"nginx", "-g", "daemon off;"},
		Environment:   []string{"NGINX_PORT=80", "TZ=UTC"},
		WorkingDir:    "/usr/share/nginx/html",
		CPU:           0.25,
		Memory:        256 << 20,
		Disk:          1 << 30,
		Network:       api.NetworkPublic,
		Ports:         []uint16{80, 443},
		RestartPolicy: "on-failure:3",
		Task:          serviceTask(),
	}
}

// jobSpec sets only what a spec always has.
func jobSpec() api.RunSpec {
	return api.RunSpec{
		Node:    "orchestrator-02",
		Name:    "snippet-q7w2e",
		Image:   "busybox:1.37",
		CPU:     1,
		Memory:  64 << 20,
		Network: api.NetworkIsolated,
		Task:    jobTask(),
	}
}

func runningRun() api.Run {
	return api.Run{
		ID:           "01926f3a8c4b7d2e9f1a3b5c7d9e0f12",
		RunSpec:      serviceSpec(),
		State:        api.StateRunning,
		RestartCount: 1,
		Endpoints: []api.Endpoint{
			{Port: 80, HostPort: 20000},
			{Port: 443, HostPort: 20001},
		},
		CreatedAt: createdAt,
		StartedAt: startedAt,
	}
}

func exitedRun() api.Run {
	return api.Run{
		ID:         "01926f3b1d2e7a4b8c5d6e7f8a9b0c1d",
		RunSpec:    jobSpec(),
		State:      api.StateExited,
		ExitCode:   137,
		Error:      "vm_lost",
		CreatedAt:  createdAt,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}
}

func createdRun() api.Run {
	return api.Run{
		ID:        "01926f3b1d2e7a4b8c5d6e7f8a9b0c1d",
		RunSpec:   jobSpec(),
		State:     api.StateCreated,
		CreatedAt: createdAt,
	}
}

func TestInfo(t *testing.T) {
	t.Parallel()

	t.Run("a service that is ready", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "info-ready.json", api.Info{
			APIVersion:          api.Version,
			ServiceVersion:      "b7c60ec1",
			MicrosandboxVersion: "0.7.6",
			Ready:               true,
			Architecture:        "amd64",
		})
	})

	t.Run("a service that is not ready says why", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "info-not-ready.json", api.Info{
			APIVersion:          api.Version,
			ServiceVersion:      "b7c60ec1",
			MicrosandboxVersion: "0.7.5",
			Reason:              "msb is 0.7.5 but the SDK is 0.7.6",
			Architecture:        "arm64",
		})
	})
}

func TestNetworkPolicy(t *testing.T) {
	t.Parallel()

	assertPinned(t, "network-policies.json", []api.NetworkPolicy{
		api.NetworkIsolated,
		api.NetworkPublic,
	})
}

func TestState(t *testing.T) {
	t.Parallel()

	assertPinned(t, "states.json", []api.State{
		api.StateCreated,
		api.StateStarting,
		api.StateRunning,
		api.StateStopping,
		api.StateRestarting,
		api.StateExited,
	})
}

func TestTask(t *testing.T) {
	t.Parallel()

	t.Run("every field", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "task-service.json", serviceTask())
	})

	t.Run("what is not set is left out, apart from the attempt", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "task-job.json", jobTask())
	})
}

func TestRunSpec(t *testing.T) {
	t.Parallel()

	t.Run("every field", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "run-spec-service.json", serviceSpec())
	})

	t.Run("what is optional is left out, and the limits never are", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "run-spec-job.json", jobSpec())
	})

	// sizes are bytes from end to end, and a float would round the largest
	// of them without saying so.
	t.Run("sizes are exact to the byte, however large", func(t *testing.T) {
		t.Parallel()

		spec := jobSpec()
		spec.Memory = math.MaxUint64
		spec.Disk = math.MaxUint64

		assertPinned(t, "run-spec-largest.json", spec)
	})
}

func TestEndpoint(t *testing.T) {
	t.Parallel()

	assertPinned(t, "endpoint.json", api.Endpoint{Port: 8080, HostPort: 20017})
}

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("a running run has its endpoints and no end", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "run-running.json", runningRun())
	})

	t.Run("an exited run says how it ended", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "run-exited.json", exitedRun())
	})

	t.Run("a run never started has no start and no end", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "run-created.json", createdRun())
	})
}

func TestRunList(t *testing.T) {
	t.Parallel()

	t.Run("runs", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "run-list.json", api.RunList{Runs: []api.Run{runningRun(), exitedRun()}})
	})

	t.Run("nothing is an empty list", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "run-list-empty.json", api.RunList{Runs: []api.Run{}})
	})
}

func TestStopRequest(t *testing.T) {
	t.Parallel()

	t.Run("a timeout", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "stop-request.json", api.StopRequest{TimeoutSeconds: 30})
	})

	t.Run("the default timeout sends nothing", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "stop-request-default.json", api.StopRequest{})
	})
}

func TestPullRequest(t *testing.T) {
	t.Parallel()

	assertPinned(t, "pull-request.json", api.PullRequest{Reference: "docker.io/library/nginx:alpine"})
}

func TestLogLine(t *testing.T) {
	t.Parallel()

	t.Run("a line keeps its nanoseconds", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "log-line.json", api.LogLine{
			Stream:  api.StreamStderr,
			At:      time.Date(2026, time.October, 4, 9, 30, 2, 1, time.UTC),
			Content: `2026/10/04 09:30:02 [notice] 1#1: using the "epoll" event method`,
		})
	})

	t.Run("streams", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "streams.json", []string{api.StreamStdout, api.StreamStderr})
	})
}

func TestStats(t *testing.T) {
	t.Parallel()

	t.Run("every counter", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "stats.json", api.Stats{
			CPUPercent:    12.5,
			MemoryUsage:   50 << 20,
			MemoryLimit:   256 << 20,
			NetworkInput:  1 << 20,
			NetworkOutput: 2 << 20,
			BlockInput:    4 << 10,
			BlockOutput:   8 << 10,
		})
	})

	t.Run("a counter at zero is still sent", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "stats-idle.json", api.Stats{})
	})
}

func TestExecRequest(t *testing.T) {
	t.Parallel()

	t.Run("a terminal", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "exec-request-terminal.json", api.ExecRequest{
			Command: []string{"/bin/sh", "-l"},
			TTY:     true,
			Env:     []string{"TERM=xterm-256color"},
			WorkDir: "/root",
			Rows:    24,
			Cols:    80,
		})
	})

	t.Run("a command is all that is needed", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "exec-request-command.json", api.ExecRequest{
			Command: []string{"cat", "/etc/os-release"},
		})
	})
}

func TestControl(t *testing.T) {
	t.Parallel()

	code := func(c int) *int { return &c }

	testcases := map[string]struct {
		file    string
		control api.Control
	}{
		"started carries the exec's id": {
			file:    "control-started.json",
			control: api.Control{Type: api.ControlStarted, ExecID: "01926f3c2e3f7b5c9d6e7f8a9b0c1d2e"},
		},
		"exit carries the exit code": {
			file:    "control-exit.json",
			control: api.Control{Type: api.ControlExit, Code: code(130)},
		},
		"an exit code of zero is still sent": {
			file:    "control-exit-zero.json",
			control: api.Control{Type: api.ControlExit, Code: code(0)},
		},
		"error carries the error": {
			file: "control-error.json",
			control: api.Control{Type: api.ControlError, Error: &api.Error{
				Code:    api.CodeNotRunning,
				Message: "run 01926f3a8c4b7d2e9f1a3b5c7d9e0f12 is not running",
			}},
		},
		"resize carries the size": {
			file:    "control-resize.json",
			control: api.Control{Type: api.ControlResize, Rows: 40, Cols: 120},
		},
		"close_stdin carries nothing": {
			file:    "control-close-stdin.json",
			control: api.Control{Type: api.ControlCloseStdin},
		},
		"signal carries the signal's number": {
			file:    "control-signal.json",
			control: api.Control{Type: api.ControlSignal, Signal: 2},
		},
	}

	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assertPinned(t, tt.file, tt.control)
		})
	}

	t.Run("types", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "control-types.json", []string{
			api.ControlStarted,
			api.ControlExit,
			api.ControlError,
			api.ControlResize,
			api.ControlCloseStdin,
			api.ControlSignal,
		})
	})
}

func TestError(t *testing.T) {
	t.Parallel()

	t.Run("an error", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "error.json", api.Error{
			Code:    api.CodeCapacity,
			Message: "the node's memory budget has 448 MiB left, and the run needs 1088 MiB",
		})
	})

	t.Run("the body of a failed request", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "error-response.json", api.ErrorResponse{Error: api.Error{
			Code:    api.CodeNotFound,
			Message: "no run 01926f3a8c4b7d2e9f1a3b5c7d9e0f12",
		}})
	})

	t.Run("codes", func(t *testing.T) {
		t.Parallel()

		assertPinned(t, "error-codes.json", []string{
			api.CodeInvalid,
			api.CodeNotSupported,
			api.CodeNotFound,
			api.CodeNameInUse,
			api.CodeNotRunning,
			api.CodeCapacity,
			api.CodePullFailed,
			api.CodeUnavailable,
			api.CodeInternal,
		})
	})

	t.Run("reads as its message", func(t *testing.T) {
		t.Parallel()

		var err error = &api.Error{Code: api.CodeCapacity, Message: "the node's memory budget is spent"}

		assert.EqualError(t, err, "the node's memory budget is spent")
	})

	t.Run("reads as its code when it has no message", func(t *testing.T) {
		t.Parallel()

		var err error = &api.Error{Code: api.CodeInternal}

		assert.EqualError(t, err, "internal")
	})
}
