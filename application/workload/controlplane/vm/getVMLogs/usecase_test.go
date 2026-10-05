package getVMLogs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	since := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	t.Run("the tail is read from the node holding the vm", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		requester := &messagingMock.Requester{Answer: func(_ context.Context, _ string, request noderequest.Request) (noderequest.Reply, error) {
			result, _ := json.Marshal([]noderequest.VMLogLine{{At: since, Source: vm.LogSourceKernel, Line: "booted"}})

			return noderequest.Reply{OK: true, Result: result, Truncated: true}, nil
		}}

		response, err := NewUseCase(w.VMs, w.Runs, requester, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "01", Since: since, Tail: 5000})
		require.NoError(t, err)
		require.Nil(t, response.NodeError)

		require.Len(t, response.Lines, 1)
		assert.Equal(t, "booted", response.Lines[0].Line)
		assert.True(t, response.Truncated)

		asked := requester.Asked()
		require.Len(t, asked, 1)
		assert.Equal(t, vmtest.Node, asked[0].NodeName)
		assert.Equal(t, noderequest.OpVMLogs, asked[0].Request.Op)

		var options noderequest.LogsRequest
		require.NoError(t, json.Unmarshal(asked[0].Request.Payload, &options))
		assert.True(t, since.Equal(options.Since))
		assert.Equal(t, uint(noderequest.MaxLogLines), options.Tail, "no more than a reply can carry")
	})

	for name, tt := range map[string]struct {
		vm     vm.VM
		answer func(context.Context, string, noderequest.Request) (noderequest.Reply, error)
		want   noderequest.Code
	}{
		"a vm on no node": {
			vm:   func() vm.VM { v := vmtest.Running("01", "owner"); v.NodeName = ""; return v }(),
			want: noderequest.CodeNotRunning,
		},
		"a node that refused": {
			vm: vmtest.Running("01", "owner"),
			answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return noderequest.Failed(vm.ErrNotRunning), nil
			},
			want: noderequest.CodeNotRunning,
		},
		"a node that did not answer in time": {
			vm: vmtest.Running("01", "owner"),
			answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return noderequest.Reply{}, context.DeadlineExceeded
			},
			want: noderequest.CodeTimeout,
		},
		"a node that is not there": {
			vm: vmtest.Running("01", "owner"),
			answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return noderequest.Reply{}, errors.New("no responders")
			},
			want: noderequest.CodeInternal,
		},
	} {
		t.Run("no lines from "+name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithVMs(tt.vm))

			response, err := NewUseCase(w.VMs, w.Runs, &messagingMock.Requester{Answer: tt.answer}, validator.New(translator.Codes{})).Execute(ctx, &Request{UUID: "01"})
			require.NoError(t, err)
			require.NotNil(t, response.NodeError)
			assert.Equal(t, tt.want, response.NodeError.Code)
		})
	}

	t.Run("somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		_, err := NewUseCase(w.VMs, w.Runs, &messagingMock.Requester{}, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "other", UUID: "01"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a vm its node has not made yet is not running, rather than not there", func(t *testing.T) {
		t.Parallel()

		// scheduled a moment ago: its node has not listed it, so the node
		// holds nothing of it and would answer that there is no such vm.
		scheduled := vmtest.Running("01", "owner")
		scheduled.CurrentState = vm.Scheduled
		scheduled.LastHeartbeatAt = time.Time{}

		w := vmtest.New(vmtest.WithVMs(scheduled))
		requester := &messagingMock.Requester{Answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
			return noderequest.Failed(domain.ErrNotExists), nil
		}}

		response, err := NewUseCase(w.VMs, w.Runs, requester, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "01"})
		require.NoError(t, err)
		require.NotNil(t, response.NodeError)
		assert.Equal(t, noderequest.CodeNotRunning, response.NodeError.Code)
		assert.Empty(t, requester.Asked(), "its node holds nothing to ask about")
	})

	t.Run("a run of the code runner's is read from what its task keeps", func(t *testing.T) {
		t.Parallel()

		run := vmtest.Run("run")
		run.ExecutionLogs = []byte("start\ntick\ntick\ndone\n")

		w := vmtest.New(vmtest.WithTasks(run))
		requester := &messagingMock.Requester{}
		useCase := NewUseCase(w.VMs, w.Runs, requester, validator.New(translator.Codes{}))

		response, err := useCase.Execute(ctx, &Request{UUID: "run"})
		require.NoError(t, err)
		require.Nil(t, response.NodeError)
		assert.Empty(t, requester.Asked(), "its node is not asked: its output is kept")

		lines := make([]string, len(response.Lines))
		for i := range response.Lines {
			lines[i] = response.Lines[i].Line
			assert.Equal(t, vm.LogSourceMain, response.Lines[i].Source)
		}

		assert.Equal(t, []string{"start", "tick", "tick", "done"}, lines)
		assert.False(t, response.Truncated)

		// what came after a line read is what comes since it, as for a VM:
		// the line read and those after it.
		since, err := useCase.Execute(ctx, &Request{UUID: "run", Since: response.Lines[2].At})
		require.NoError(t, err)
		require.Len(t, since.Lines, 2)
		assert.Equal(t, "done", since.Lines[1].Line)

		tail, err := useCase.Execute(ctx, &Request{UUID: "run", Tail: 1})
		require.NoError(t, err)
		require.Len(t, tail.Lines, 1)
		assert.Equal(t, "done", tail.Lines[0].Line)
	})

	t.Run("a run is nobody's own to read", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))

		_, err := NewUseCase(w.VMs, w.Runs, &messagingMock.Requester{}, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "run"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
