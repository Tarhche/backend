package startStack

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

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		state   stack.State
		next    stack.State
		action  stack.Action
		refused domain.ValidationErrors
	}{
		"a stopped stack is started":             {state: stack.Stopped, next: stack.Starting, action: stack.ActionStart},
		"a failed one is deployed again":         {state: stack.Failed, next: stack.Deploying, action: stack.ActionUp},
		"a running one is where it was asked":    {state: stack.Running, next: stack.Running},
		"one being stopped is busy":              {state: stack.Stopping, next: stack.Stopping, refused: domain.ValidationErrors{"stack": "invalid_state_transition"}},
		"one on its way out is not brought back": {state: stack.Removing, next: stack.Removing, refused: domain.ValidationErrors{"stack": "invalid_state_transition"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(
				vmtest.WithVMs(vmtest.Docker("01", "owner")),
				vmtest.WithStacks(stack.Stack{UUID: "s1", OwnerUUID: "owner", VMUUID: "01", Slug: "web-abcde", State: tt.state}),
			)

			response, err := NewUseCase(w.Stacks, w.VMs, dispatch.New(w.Stacks, w.Producer, slog.New(slog.DiscardHandler)), validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "s1"})
			require.NoError(t, err)
			assert.Equal(t, tt.refused, response.ValidationErrors)

			stored, _ := w.Stacks.Stored("s1")
			assert.Equal(t, tt.next.String(), stored.State.String())

			if len(tt.action) == 0 {
				assert.Empty(t, w.Producer.Messages())

				return
			}

			var asked events.StackRequested
			require.True(t, w.Producer.Last(events.StackRequestedName, &asked))
			assert.Equal(t, tt.action, asked.Action)
			assert.Equal(t, "web-abcde", asked.Project)
		})
	}

	t.Run("one in a vm that is not running is refused", func(t *testing.T) {
		t.Parallel()

		stopped := vmtest.Docker("01", "owner")
		stopped.CurrentState = vm.Stopped

		w := vmtest.New(vmtest.WithVMs(stopped), vmtest.WithStacks(stack.Stack{UUID: "s1", VMUUID: "01", State: stack.Failed}))

		response, err := NewUseCase(w.Stacks, w.VMs, dispatch.New(w.Stacks, w.Producer, slog.New(slog.DiscardHandler)), validator.New(translator.Codes{})).Execute(ctx, &Request{UUID: "s1"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "vm_not_running"}, response.ValidationErrors)
	})
}
