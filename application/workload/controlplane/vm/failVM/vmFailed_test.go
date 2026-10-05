package failVM

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

func failure(t *testing.T, at time.Time, reason string) []byte {
	t.Helper()

	payload, err := json.Marshal(events.VMFailed{VMUUID: "01", NodeName: vmtest.Node, Reason: reason, At: at})
	require.NoError(t, err)

	return payload
}

func TestVMFailed_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		vm       vm.VM
		at       time.Time
		reason   string
		state    vm.State
		expected vm.State
		why      string
	}{
		"a vm wanted running is failed and given up on": {
			vm: func() vm.VM {
				v := vmtest.Running("01", "owner")
				v.CurrentState = vm.Scheduled
				v.UpdatedAt = time.Now()
				return v
			}(),
			at:       time.Now(),
			reason:   "pull access denied for nosuchimage",
			state:    vm.Failed,
			expected: vm.Failed,
			why:      "pull access denied for nosuchimage",
		},
		"one wanted stopped is failed, and still wanted stopped": {
			vm: func() vm.VM {
				v := vmtest.Running("01", "owner")
				v.CurrentState = vm.Stopping
				v.ExpectedState = vm.Stopped
				v.UpdatedAt = time.Now()
				return v
			}(),
			at:       time.Now(),
			state:    vm.Failed,
			expected: vm.Stopped,
			why:      defaultReason,
		},
		"a failure of what was asked before the latest request is not the latest's": {
			vm: func() vm.VM {
				v := vmtest.Running("01", "owner")
				v.CurrentState = vm.Starting
				v.UpdatedAt = time.Now()
				return v
			}(),
			at:       time.Now().Add(-time.Minute),
			reason:   "an old failure",
			state:    vm.Starting,
			expected: vm.Running,
		},
		"one on its way out stays on its way out": {
			vm: func() vm.VM {
				v := vmtest.Running("01", "owner")
				v.CurrentState = vm.Deleting
				v.ExpectedState = vm.Deleting
				return v
			}(),
			at:       time.Now(),
			reason:   "the engine is away",
			state:    vm.Deleting,
			expected: vm.Deleting,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithVMs(tt.vm))

			require.NoError(t, NewVMFailed(w.VMs, slog.New(slog.DiscardHandler)).Handle(ctx, failure(t, tt.at, tt.reason)))

			stored, _ := w.VMs.Stored("01")
			assert.Equal(t, tt.state, stored.CurrentState)
			assert.Equal(t, tt.expected, stored.ExpectedState)
			assert.Equal(t, tt.why, stored.Reason)
		})
	}

	t.Run("what will never be handled is not handed back", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()
		handler := NewVMFailed(w.VMs, slog.New(slog.DiscardHandler))

		assert.NoError(t, handler.Handle(ctx, []byte("{")))
		assert.NoError(t, handler.Handle(ctx, failure(t, time.Now(), "for a vm nobody has")))
	})
}
