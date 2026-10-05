package stopVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("the owner's vm is asked", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		response, err := NewUseCase(w.VMs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "01"})
		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, vm.Stopping, stored.CurrentState)
		assert.Equal(t, []string{events.VMStopRequestedName}, w.Producer.Subjects())
	})

	t.Run("somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		_, err := NewUseCase(w.VMs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{OwnerUUID: "other", UUID: "01"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, w.Producer.Messages())
	})

	t.Run("one on its way out is refused", func(t *testing.T) {
		t.Parallel()

		deleting := vmtest.Running("01", "owner")
		deleting.CurrentState = vm.Deleting

		w := vmtest.New(vmtest.WithVMs(deleting))

		response, err := NewUseCase(w.VMs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{UUID: "01"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "invalid_state_transition"}, response.ValidationErrors)
	})

	t.Run("a vm has to be named", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		response, err := NewUseCase(w.VMs, w.Lifecycle, validator.New(translator.Codes{})).Execute(ctx, &Request{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"uuid": "required_field"}, response.ValidationErrors)
	})
}
