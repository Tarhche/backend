package answerRequest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/getVMLogs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	mocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

type one struct {
	daemon docker.Daemon
}

func (o one) Daemon(string) docker.Daemon {
	return o.daemon
}

// timed keeps which operations were timed.
type timed struct {
	lock sync.Mutex
	ops  []string
}

func (r *timed) DockerRequest(_ context.Context, op string, _ time.Duration) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.ops = append(r.ops, op)
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	e := memory.New()
	_, err := e.Create(t.Context(), vm.Spec{ID: "vm-1", Kind: vm.KindMachine, Image: "ubuntu:24.04"})
	require.NoError(t, err)
	require.NoError(t, e.Log("vm-1", vm.LogSourceKernel, "booted"))

	logs := getVMLogs.NewUseCase(e, validates{})

	testcases := []struct {
		name    string
		request noderequest.Request
		expect  func(daemon *mocks.MockDaemon)

		wantResult string
		wantCode   noderequest.Code
		wantTimed  []string
	}{
		{
			name:    "a request to a Docker VM waits for its dockerd, then is answered",
			request: noderequest.Request{Op: noderequest.OpContainersList, VMUUID: "vm-1"},
			expect: func(d *mocks.MockDaemon) {
				d.On("Ping", mock.Anything).Return(nil).Once()
				d.On("Containers", mock.Anything, docker.ContainerFilter{}).Return([]docker.Container{{ID: "c1", Name: "web"}}, nil)
			},
			wantResult: `[{"id":"c1","name":"web","image":"","state":"","status":"","command":"","ports":null,"networks":null,"mounts":null,"restart_policy":"","created_at":"0001-01-01T00:00:00Z"}]`,
			wantTimed:  []string{"docker.containers.list"},
		},
		{
			name:    "a ping is answered once dockerd answers",
			request: noderequest.Request{Op: noderequest.OpPing, VMUUID: "vm-1"},
			expect:  func(d *mocks.MockDaemon) { d.On("Ping", mock.Anything).Return(nil).Once() },
			wantTimed: []string{
				"docker.ping",
			},
		},
		{
			name:    "a dockerd that did not come up in time is unavailable",
			request: noderequest.Request{Op: noderequest.OpImagesList, VMUUID: "vm-1"},
			expect: func(d *mocks.MockDaemon) {
				d.On("Ping", mock.Anything).Return(fmt.Errorf("%w: its dockerd did not answer in 3m0s", docker.ErrUnavailable))
			},
			wantCode:  noderequest.CodeDockerUnavailable,
			wantTimed: []string{"docker.images.list"},
		},
		{
			name:    "a VM with no docker in it is said to be no Docker VM",
			request: noderequest.Request{Op: noderequest.OpImagesList, VMUUID: "vm-1"},
			expect: func(d *mocks.MockDaemon) {
				d.On("Ping", mock.Anything).Return(vm.ErrNotDocker)
			},
			wantCode:  noderequest.CodeNotDocker,
			wantTimed: []string{"docker.images.list"},
		},
		{
			name:    "a stopped VM is said to be not running",
			request: noderequest.Request{Op: noderequest.OpVolumesList, VMUUID: "vm-1"},
			expect: func(d *mocks.MockDaemon) {
				d.On("Ping", mock.Anything).Return(vm.ErrNotRunning)
			},
			wantCode:  noderequest.CodeNotRunning,
			wantTimed: []string{"docker.volumes.list"},
		},
		{
			name:       "a VM's log is read without asking any dockerd",
			request:    noderequest.Request{Op: noderequest.OpVMLogs, VMUUID: "vm-1", Payload: json.RawMessage(`{"tail":5}`)},
			wantResult: `[{"at":"` + mustLogTime(t, e) + `","source":"kernel","line":"booted"}]`,
		},
		{
			name:     "the log of a VM that is not here is not found",
			request:  noderequest.Request{Op: noderequest.OpVMLogs, VMUUID: "vm-2"},
			wantCode: noderequest.CodeNotFound,
		},
		{
			name:     "an operation nobody knows is refused",
			request:  noderequest.Request{Op: "docker.containers.exec", VMUUID: "vm-1"},
			wantCode: noderequest.CodeInvalid,
		},
		{
			name:     "a request about no VM is refused",
			request:  noderequest.Request{Op: noderequest.OpContainersList},
			wantCode: noderequest.CodeInvalid,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var daemon mocks.MockDaemon
			if tt.expect != nil {
				tt.expect(&daemon)
			}
			defer daemon.AssertExpectations(t)

			recorder := &timed{}

			reply := NewUseCase(one{daemon: &daemon}, logs, recorder).Handle(t.Context(), tt.request)

			assert.Equal(t, tt.wantTimed, recorder.ops)

			if len(tt.wantCode) > 0 {
				assert.False(t, reply.OK)
				require.NotNil(t, reply.Error)
				assert.Equal(t, tt.wantCode, reply.Error.Code)

				return
			}

			assert.True(t, reply.OK)
			assert.Nil(t, reply.Error)

			if len(tt.wantResult) == 0 {
				assert.Empty(t, reply.Result)

				return
			}

			assert.JSONEq(t, tt.wantResult, string(reply.Result))
		})
	}
}

// mustLogTime is when the only line of vm-1's log was written, as it travels.
func mustLogTime(t *testing.T, e *memory.Engine) string {
	t.Helper()

	lines, err := e.Logs(t.Context(), "vm-1", vm.LogOptions{})
	require.NoError(t, err)
	require.Len(t, lines, 1)

	encoded, err := json.Marshal(lines[0].At)
	require.NoError(t, err)

	var at string
	require.NoError(t, json.Unmarshal(encoded, &at))

	return at
}
