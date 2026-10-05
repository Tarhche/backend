package beatHeart

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/getEndpoint"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/attachTask"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/runTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/task/vmruntime"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// lastBeat is what the node last said about the task.
func lastBeat(t *testing.T, producer *messaging.Recorder) events.Heartbeat {
	t.Helper()

	beats, err := messaging.Produced[events.Heartbeat](producer, events.HeartbeatName)
	require.NoError(t, err)
	require.NotEmpty(t, beats)

	return beats[len(beats)-1]
}

// TestUseCase_Execute_onVMs follows a live code-runner snippet through what
// the node does with it, on the VM runtime: it is run, reported running with
// the port it serves, reached on that port, given a terminal, and reported
// again once its code has run to its end, with what it printed.
func TestUseCase_Execute_onVMs(t *testing.T) {
	t.Parallel()

	exit := make(chan int)

	e := memory.New(
		memory.WithHost("vmhost-01"),

		// the snippet's code prints, and runs until the test says it ends.
		memory.WithMain(func(ctx context.Context, spec vm.Spec, log func(string)) int {
			log("listening on 3000")

			select {
			case code := <-exit:
				log("done")

				return code
			case <-ctx.Done():
				return -1
			}
		}),

		// its terminal is a shell that says back what is typed.
		memory.WithExec(func(_ context.Context, _ string, _ vm.ExecOptions, stdin io.Reader, stdout io.Writer, _ io.Writer) int {
			read, _ := io.ReadAll(stdin)
			_, _ = io.WriteString(stdout, strings.ToUpper(string(read)))

			return 0
		}),
	)

	runtime := vmruntime.New(e, discardLogger())
	producer := &messaging.Recorder{}
	heartbeat := NewUseCase(runtime, producer, nodeName, discardLogger())

	// run: what the control plane scheduled on this node.
	ran, err := runTask.NewUseCase(runtime, validates{}, nodeName).Execute(t.Context(), &runTask.Request{
		UUID:          "task-uuid",
		Name:          "a-request-id",
		Slug:          "snippet-abcde",
		Kind:          task.KindJob,
		OwnerUUID:     "guest",
		Image:         "ghcr.io/tarhche/code-runner:nodejs-22.14",
		Command:       []string{"--timeout", "120", "require('http').createServer().listen(3000)"},
		ExposedPorts:  []port.Port{3000},
		NetworkPolicy: network.PolicyIsolated,
		Interactive:   true,
		TTL:           2 * time.Minute,
		ResourceLimits: runTask.ResourceLimits{
			Cpu:    2,
			Memory: 200 << 20,
			Disk:   100 << 20,
		},
	})
	require.NoError(t, err)
	require.Empty(t, ran.ValidationErrors)

	// heartbeat: it is running, served on its port, and counting down.
	require.NoError(t, heartbeat.Execute(t.Context()))

	running := lastBeat(t, producer)
	assert.Equal(t, "task-uuid", running.UUID)
	assert.Equal(t, int(task.Running), running.State)
	assert.Equal(t, []events.Endpoint{{TaskPort: 3000, HostPort: 20000}}, running.Endpoints)
	assert.False(t, running.Deadline.IsZero(), "a snippet being watched is told when it will be stopped")

	// its port, reached by the slug the ingress asks for.
	endpoint, err := getEndpoint.NewUseCase(e).Execute(t.Context(), &getEndpoint.Request{Slug: "snippet-abcde"})
	require.NoError(t, err)
	assert.Equal(t, "vmhost-01:20000", endpoint.Address)

	// its terminal, opened for whoever it belongs to.
	session, validationErrors, err := attachTask.NewUseCase(runtime, validates{}).Execute(t.Context(), &attachTask.Request{
		UUID:      "task-uuid",
		TTY:       true,
		OwnerUUID: "guest",
	})
	require.NoError(t, err)
	require.Empty(t, validationErrors)

	output := make(chan string, 1)
	go func() {
		said, _ := io.ReadAll(session)
		output <- string(said)
	}()

	_, err = io.WriteString(session, "ls\n")
	require.NoError(t, err)
	require.NoError(t, session.Close())
	assert.Contains(t, []string{"LS\n", ""}, <-output, "what the terminal said, if it said it before it was closed")

	// the code ends, and the next beat says so, with what it printed.
	exit <- 0

	require.Eventually(t, func() bool {
		require.NoError(t, heartbeat.Execute(t.Context()))

		return lastBeat(t, producer).State == int(task.Completed)
	}, 5*time.Second, 10*time.Millisecond)

	completed := lastBeat(t, producer)
	assert.Equal(t, "listening on 3000\ndone\n", string(completed.Logs))
	assert.Empty(t, completed.Endpoints, "nothing is served by a VM that has stopped")
}
