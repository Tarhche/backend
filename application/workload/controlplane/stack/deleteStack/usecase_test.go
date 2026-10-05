package deleteStack

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func deployed(state stack.State) stack.Stack {
	return stack.Stack{UUID: "s1", OwnerUUID: "owner", VMUUID: "01", Slug: "web-abcde", State: state, ExpectedState: stack.Running}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		vm      *vm.VM
		stack   stack.Stack
		pending bool
		gone    bool
		refused domain.ValidationErrors
	}{
		"a running stack is taken down, and goes once it is": {
			vm:      func() *vm.VM { v := vmtest.Docker("01", "owner"); return &v }(),
			stack:   deployed(stack.Running),
			pending: true,
		},
		"one whose vm is gone goes at once": {
			stack: deployed(stack.Running),
			gone:  true,
		},
		"one whose vm failed goes at once": {
			vm:    func() *vm.VM { v := vmtest.Docker("01", "owner"); v.CurrentState = vm.Failed; return &v }(),
			stack: deployed(stack.Running),
			gone:  true,
		},
		"one still waiting for its vm goes at once": {
			vm:    func() *vm.VM { v := vmtest.Docker("01", "owner"); v.CurrentState = vm.Scheduled; return &v }(),
			stack: func() stack.Stack { s := deployed(stack.Deploying); s.Reason = dispatch.ReasonWaitingForVM; return s }(),
			gone:  true,
		},
		"one in a vm that is stopped waits for it to be started": {
			vm: func() *vm.VM {
				v := vmtest.Docker("01", "owner")
				v.CurrentState = vm.Stopped
				v.ExpectedState = vm.Stopped
				return &v
			}(),
			stack:   deployed(stack.Running),
			refused: domain.ValidationErrors{"vm": "vm_not_running"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts := []vmtest.Option{vmtest.WithStacks(tt.stack)}
			if tt.vm != nil {
				opts = append(opts, vmtest.WithVMs(*tt.vm))
			}

			w := vmtest.New(opts...)

			response, err := NewUseCase(w.Stacks, w.VMs, dispatch.New(w.Stacks, w.Producer, slog.New(slog.DiscardHandler)), validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "s1", RemoveVolumes: true})
			require.NoError(t, err)
			assert.Equal(t, tt.refused, response.ValidationErrors)
			assert.Equal(t, tt.pending, response.Pending)

			stored, kept := w.Stacks.Stored("s1")
			assert.Equal(t, !tt.gone, kept)

			if tt.pending {
				assert.Equal(t, stack.Removing, stored.State)

				var asked events.StackRequested
				require.True(t, w.Producer.Last(events.StackRequestedName, &asked))
				assert.Equal(t, stack.ActionDown, asked.Action)
				assert.True(t, asked.RemoveVolumes)
			} else {
				assert.Empty(t, w.Producer.Produced())
			}
		})
	}

	t.Run("somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithStacks(deployed(stack.Running)))

		_, err := NewUseCase(w.Stacks, w.VMs, dispatch.New(w.Stacks, w.Producer, slog.New(slog.DiscardHandler)), validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "other", UUID: "s1"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
