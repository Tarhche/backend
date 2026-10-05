package containers

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	mocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
)

// one is a node with one Docker VM, whose dockerd is daemon.
type one struct {
	daemon docker.Daemon
}

func (o one) Daemon(string) docker.Daemon {
	return o.daemon
}

func request(t *testing.T, op noderequest.Op, payload any) noderequest.Request {
	t.Helper()

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)

	return noderequest.Request{Op: op, VMUUID: "vm-1", Payload: encoded}
}

var web = docker.Container{ID: "c1", Name: "web", Image: "nginx:alpine", State: "running", Stack: "shop", Service: "web"}

func TestUseCase_Answer(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		request func(t *testing.T) noderequest.Request
		expect  func(daemon *mocks.MockDaemon)

		want          any
		wantTruncated bool
		wantCode      noderequest.Code
	}{
		{
			name: "a listing is narrowed as asked",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersList, noderequest.ContainersRequest{All: true, Stack: "shop"})
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("Containers", mock.Anything, docker.ContainerFilter{All: true, Stack: "shop"}).Return([]docker.Container{web}, nil)
			},
			want: []noderequest.Container{noderequest.NewContainer(web)},
		},
		{
			name: "a listing asked with no payload is of the running ones",
			request: func(t *testing.T) noderequest.Request {
				return noderequest.Request{Op: noderequest.OpContainersList, VMUUID: "vm-1"}
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("Containers", mock.Anything, docker.ContainerFilter{}).Return([]docker.Container{}, nil)
			},
			want: []noderequest.Container{},
		},
		{
			name: "one container is inspected",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersInspect, noderequest.ContainerRequest{ID: "c1"})
			},
			expect: func(d *mocks.MockDaemon) { d.On("Container", mock.Anything, "c1").Return(web, nil) },
			want:   noderequest.NewContainer(web),
		},
		{
			name: "a container is created from what was asked",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersCreate, noderequest.ContainerSpec{Image: "nginx:alpine", Name: "web"})
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("CreateContainer", mock.Anything, docker.ContainerSpec{Image: "nginx:alpine", Name: "web"}).Return(web, nil)
			},
			want: noderequest.NewContainer(web),
		},
		{
			name: "a container is not created from no image",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersCreate, noderequest.ContainerSpec{Name: "web"})
			},
			wantCode: noderequest.CodeInvalid,
		},
		{
			name: "a container is started",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersStart, noderequest.ContainerRequest{ID: "c1"})
			},
			expect: func(d *mocks.MockDaemon) { d.On("StartContainer", mock.Anything, "c1").Return(nil) },
		},
		{
			name: "a container is stopped",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersStop, noderequest.ContainerRequest{ID: "c1"})
			},
			expect: func(d *mocks.MockDaemon) { d.On("StopContainer", mock.Anything, "c1").Return(nil) },
		},
		{
			name: "a container is restarted",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersRestart, noderequest.ContainerRequest{ID: "c1"})
			},
			expect: func(d *mocks.MockDaemon) { d.On("RestartContainer", mock.Anything, "c1").Return(nil) },
		},
		{
			name: "a container is removed, by force when asked",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersRemove, noderequest.RemoveRequest{ID: "c1", Force: true})
			},
			expect: func(d *mocks.MockDaemon) { d.On("RemoveContainer", mock.Anything, "c1", true).Return(nil) },
		},
		{
			name: "a log longer than a reply carries is its last lines, and says so",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersLogs, noderequest.ContainerLogsRequest{ID: "c1"})
			},
			expect: func(d *mocks.MockDaemon) {
				lines := make([]docker.LogLine, noderequest.MaxLogLines+1)
				for n := range lines {
					lines[n] = docker.LogLine{Stream: "stdout", Line: fmt.Sprintf("line %d", n)}
				}

				d.On("ContainerLogs", mock.Anything, "c1", docker.LogOptions{Tail: noderequest.MaxLogLines + 1}).Return(lines, nil)
			},
			want: func() []noderequest.LogLine {
				lines := make([]noderequest.LogLine, noderequest.MaxLogLines)
				for n := range lines {
					lines[n] = noderequest.LogLine{Stream: "stdout", Line: fmt.Sprintf("line %d", n+1)}
				}

				return lines
			}(),
			wantTruncated: true,
		},
		{
			name: "the last lines asked for are what they are",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersLogs, noderequest.ContainerLogsRequest{ID: "c1", Tail: 2, Since: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)})
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("ContainerLogs", mock.Anything, "c1", docker.LogOptions{Tail: 2, Since: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)}).
					Return([]docker.LogLine{{Stream: "stderr", Line: "warning"}}, nil)
			},
			want: []noderequest.LogLine{{Stream: "stderr", Line: "warning"}},
		},
		{
			name: "what a container uses is sampled",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersStats, noderequest.ContainerRequest{ID: "c1"})
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("ContainerStats", mock.Anything, "c1").Return(docker.Stats{CPUPercent: 12.5, PIDs: 3}, nil)
			},
			want: noderequest.Stats{CPUPercent: 12.5, PIDs: 3},
		},
		{
			name: "a container is attached to a network under its aliases",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersConnect, noderequest.ConnectRequest{Network: "backend", Container: "c1", Aliases: []string{"cache"}})
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("ConnectNetwork", mock.Anything, "backend", "c1", []string{"cache"}).Return(nil)
			},
		},
		{
			name: "a container is detached from a network",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersDisconnect, noderequest.DisconnectRequest{Network: "backend", Container: "c1"})
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("DisconnectNetwork", mock.Anything, "backend", "c1", false).Return(nil)
			},
		},
		{
			name: "attaching needs a network",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersConnect, noderequest.ConnectRequest{Container: "c1"})
			},
			wantCode: noderequest.CodeInvalid,
		},
		{
			name: "naming no container is refused",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersStart, noderequest.ContainerRequest{})
			},
			wantCode: noderequest.CodeInvalid,
		},
		{
			name: "a payload that cannot be read is refused",
			request: func(t *testing.T) noderequest.Request {
				return noderequest.Request{Op: noderequest.OpContainersInspect, VMUUID: "vm-1", Payload: json.RawMessage(`[1]`)}
			},
			wantCode: noderequest.CodeInvalid,
		},
		{
			name: "a container that is not there is not found",
			request: func(t *testing.T) noderequest.Request {
				return request(t, noderequest.OpContainersInspect, noderequest.ContainerRequest{ID: "c9"})
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("Container", mock.Anything, "c9").Return(docker.Container{}, fmt.Errorf("%w: No such container: c9", domain.ErrNotExists))
			},
			wantCode: noderequest.CodeNotFound,
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

			result, truncated, err := NewUseCase(one{daemon: &daemon}).Answer(t.Context(), tt.request(t))
			if len(tt.wantCode) > 0 {
				assert.Equal(t, tt.wantCode, noderequest.ErrorOf(err).Code)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
			assert.Equal(t, tt.wantTruncated, truncated)
		})
	}
}
