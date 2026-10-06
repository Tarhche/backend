package requestDocker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("the question goes to the node holding the vm, and its answer comes back", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))

		requester := &messagingMock.Requester{Answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
			return noderequest.Reply{OK: true, Result: json.RawMessage(`[{"id":"c1","name":"web"}]`), Truncated: true}, nil
		}}

		response, err := NewUseCase(w.Entities, requester, validator.New(translator.Codes{})).Execute(ctx, &Request{
			OwnerUUID: "owner",
			VMUUID:    "01",
			Op:        noderequest.OpContainersList,
			Payload:   json.RawMessage(`{"all":true}`),
		})
		require.NoError(t, err)
		require.Nil(t, response.NodeError)

		assert.JSONEq(t, `[{"id":"c1","name":"web"}]`, string(response.Result))
		assert.True(t, response.Truncated)

		asked := requester.Asked()
		require.Len(t, asked, 1)
		assert.Equal(t, vmtest.Node, asked[0].NodeName)
		assert.Equal(t, noderequest.Request{Op: noderequest.OpContainersList, VMUUID: "01", Payload: json.RawMessage(`{"all":true}`)}, asked[0].Request)
	})

	t.Run("a vm still coming up is asked all the same: its node waits for dockerd", func(t *testing.T) {
		t.Parallel()

		booting := vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Scheduled })

		w := vmtest.New(vmtest.WithVMs(booting))
		requester := &messagingMock.Requester{}

		response, err := NewUseCase(w.Entities, requester, validator.New(translator.Codes{})).Execute(ctx, &Request{VMUUID: "01", Op: noderequest.OpPing})
		require.NoError(t, err)
		assert.Nil(t, response.NodeError)
		assert.Len(t, requester.Asked(), 1)
	})

	t.Run("a vm its node has not made yet is not running, rather than not there", func(t *testing.T) {
		t.Parallel()

		// scheduled a moment ago: its node has not listed it, so the node
		// holds nothing of it to wait for, and would answer that there is no
		// such vm, which the blog would take for the vm being gone.
		scheduled := vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) {
			v.Status.State = vmKind.Scheduled
			v.Status.ObservedAt = time.Time{}
		})

		w := vmtest.New(vmtest.WithVMs(scheduled))
		requester := &messagingMock.Requester{Answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
			return noderequest.Failed(domain.ErrNotExists), nil
		}}

		response, err := NewUseCase(w.Entities, requester, validator.New(translator.Codes{})).Execute(ctx, &Request{VMUUID: "01", Op: noderequest.OpContainersList})
		require.NoError(t, err)
		require.NotNil(t, response.NodeError)
		assert.Equal(t, noderequest.CodeNotRunning, response.NodeError.Code)
		assert.Empty(t, requester.Asked(), "its node holds nothing to ask about")
	})

	for name, tt := range map[string]struct {
		vm     vmKind.VM
		answer func(context.Context, string, noderequest.Request) (noderequest.Reply, error)
		want   error
	}{
		"a vm that is not a docker vm": {
			vm:   vmtest.Running("01", "owner"),
			want: vm.ErrNotDocker,
		},
		"a docker vm that failed": {
			vm:   vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Failed }),
			want: vm.ErrNotRunning,
		},
		"a docker vm that is stopped": {
			vm:   vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped }),
			want: vm.ErrNotRunning,
		},
		"a docker vm on no node": {
			vm:   vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Metadata.Node = "" }),
			want: vm.ErrNotRunning,
		},
		"a dockerd that did not come up": {
			vm: vmtest.Docker("01", "owner"),
			answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return noderequest.Failed(docker.ErrUnavailable), nil
			},
			want: docker.ErrUnavailable,
		},
		"a container that is not there": {
			vm: vmtest.Docker("01", "owner"),
			answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return noderequest.Failed(domain.ErrNotExists), nil
			},
			want: domain.ErrNotExists,
		},
		"a node that did not answer in time": {
			vm: vmtest.Docker("01", "owner"),
			answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return noderequest.Reply{}, context.DeadlineExceeded
			},
			want: context.DeadlineExceeded,
		},
	} {
		t.Run("no answer from "+name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithVMs(tt.vm))

			response, err := NewUseCase(w.Entities, &messagingMock.Requester{Answer: tt.answer}, validator.New(translator.Codes{})).Execute(ctx, &Request{
				VMUUID: "01",
				Op:     noderequest.OpContainersInspect,
			})
			require.NoError(t, err)
			require.NotNil(t, response.NodeError)
			assert.ErrorIs(t, response.NodeError, tt.want)
		})
	}

	t.Run("only dockerd's operations go through here", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))

		response, err := NewUseCase(w.Entities, &messagingMock.Requester{}, validator.New(translator.Codes{})).Execute(ctx, &Request{VMUUID: "01", Op: kind.Op(vmKind.Name, vmKind.ActionLogs)})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"op": "invalid_value"}, response.ValidationErrors)
	})

	t.Run("somebody else's vm is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))

		_, err := NewUseCase(w.Entities, &messagingMock.Requester{}, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "other", VMUUID: "01", Op: noderequest.OpPing})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
