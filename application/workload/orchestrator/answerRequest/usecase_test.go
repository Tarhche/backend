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

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	mocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
)

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
			name:     "a VM's log is the vm kind's to answer, and no operation of the node's own",
			request:  noderequest.Request{Op: "vm.logs", VMUUID: "vm-1", Payload: json.RawMessage(`{"tail":5}`)},
			wantCode: noderequest.CodeInvalid,
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

			reply := NewUseCase(one{daemon: &daemon}, recorder).Handle(t.Context(), tt.request)

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
