package deleteVM

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// VMDeleted forgets a VM its node says it no longer holds, without waiting for
// a heartbeat to leave it out.
//
// Only a VM that was asked to go is forgotten: one that vanished from a node
// without being asked is the heartbeat's to notice and bring back.
type VMDeleted struct {
	vmRepository vm.Repository
	lifecycle    *lifecycle.Lifecycle
	logger       *slog.Logger
}

var _ domain.MessageHandler = &VMDeleted{}

func NewVMDeleted(vmRepository vm.Repository, lifecycle *lifecycle.Lifecycle, logger *slog.Logger) *VMDeleted {
	return &VMDeleted{vmRepository: vmRepository, lifecycle: lifecycle, logger: logger}
}

// Handle never fails for a message that will never be handled: redelivering it
// would only fail the same way, at once and for ever.
func (h *VMDeleted) Handle(ctx context.Context, data []byte) error {
	var deleted events.VMDeleted
	if err := json.Unmarshal(data, &deleted); err != nil {
		h.logger.ErrorContext(ctx, "a vm deletion that cannot be read", "error", err)

		return nil
	}

	v, err := h.vmRepository.GetOne(ctx, deleted.VMUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	if v.CurrentState != vm.Deleting {
		return nil
	}

	return h.lifecycle.Forget(ctx, v.UUID)
}
