package getStack

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	web := stack.Stack{UUID: "s1", OwnerUUID: "owner", VMUUID: "01", Slug: "web-abcde", State: stack.Running}

	t.Run("the stack and the containers compose made for it", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")), vmtest.WithStacks(web))

		var filter noderequest.ContainersRequest
		requester := &messagingMock.Requester{Answer: func(_ context.Context, _ string, request noderequest.Request) (noderequest.Reply, error) {
			_ = json.Unmarshal(request.Payload, &filter)
			result, _ := json.Marshal([]noderequest.Container{{ID: "c1", Name: "web-abcde-web-1", Stack: "web-abcde", Service: "web"}})

			return noderequest.Reply{OK: true, Result: result}, nil
		}}

		response, err := NewUseCase(w.Stacks, w.VMs, requester, slog.New(slog.DiscardHandler)).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "s1"})
		require.NoError(t, err)

		assert.Equal(t, "s1", response.UUID)
		assert.False(t, response.VMNotRunning)
		require.Len(t, response.Containers, 1)
		assert.Equal(t, "web", response.Containers[0].Service)
		assert.Equal(t, noderequest.ContainersRequest{All: true, Stack: "web-abcde"}, filter, "only the project's, stopped ones too")
	})

	t.Run("one whose vm is not running has none to show, and says why", func(t *testing.T) {
		t.Parallel()

		stopped := vmtest.Docker("01", "owner")
		stopped.CurrentState = vm.Stopped

		w := vmtest.New(vmtest.WithVMs(stopped), vmtest.WithStacks(web))
		requester := &messagingMock.Requester{}

		response, err := NewUseCase(w.Stacks, w.VMs, requester, slog.New(slog.DiscardHandler)).Execute(ctx, &Request{UUID: "s1"})
		require.NoError(t, err)

		assert.True(t, response.VMNotRunning)
		assert.Empty(t, response.Containers)
		assert.NotNil(t, response.Containers, "none, rather than nothing")
		assert.Empty(t, requester.Asked())
	})

	t.Run("one whose dockerd did not come up says so too", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")), vmtest.WithStacks(web))
		requester := &messagingMock.Requester{Answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
			return noderequest.Reply{Error: &noderequest.Error{Code: noderequest.CodeDockerUnavailable}}, nil
		}}

		response, err := NewUseCase(w.Stacks, w.VMs, requester, slog.New(slog.DiscardHandler)).Execute(ctx, &Request{UUID: "s1"})
		require.NoError(t, err)
		assert.True(t, response.VMNotRunning)
	})

	t.Run("somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithStacks(web))

		_, err := NewUseCase(w.Stacks, w.VMs, &messagingMock.Requester{}, slog.New(slog.DiscardHandler)).Execute(ctx, &Request{OwnerUUID: "other", UUID: "s1"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
