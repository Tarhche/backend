package restoreVM

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/command"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// VMRestored ends the restore a VM was waiting on.
//
// The node made the VM again from the archive, which boots it. One that was
// wanted stopped is stopped again; one wanted running is starting, and its
// next heartbeat says when it is up.
type VMRestored struct {
	vmRepository vm.Repository
	lifecycle    *lifecycle.Lifecycle
	commander    *command.Commander
	logger       *slog.Logger
}

var _ domain.MessageHandler = &VMRestored{}

func NewVMRestored(vmRepository vm.Repository, lifecycle *lifecycle.Lifecycle, commander *command.Commander, logger *slog.Logger) *VMRestored {
	return &VMRestored{vmRepository: vmRepository, lifecycle: lifecycle, commander: commander, logger: logger}
}

// Handle never fails for a message that will never be handled: redelivering it
// would only fail the same way, at once and for ever.
func (h *VMRestored) Handle(ctx context.Context, data []byte) error {
	var restored events.VMRestored
	if err := json.Unmarshal(data, &restored); err != nil {
		h.logger.ErrorContext(ctx, "a vm restore that cannot be read", "error", err)

		return nil
	}

	v, err := h.vmRepository.GetOne(ctx, restored.VMUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	// a restore this VM is no longer waiting on: it was asked for something
	// else since, or this one was reported already.
	if v.RestoreFrom != restored.SnapshotUUID {
		return nil
	}

	// a VM made from a snapshot is made with its disk, which is said the same
	// way; it is on its way up as it was, and its node reports when it is.
	// Making it again is no longer making it from the snapshot.
	if v.CurrentState != vm.Restoring {
		v.RestoreFrom = ""
		_, err := h.vmRepository.Save(ctx, &v)

		return err
	}

	v.RestoreFrom = ""
	v.Reason = ""

	// moved on, so that a heartbeat taken before the restore ended cannot be
	// read as what the VM is now.
	v.UpdatedAt = h.lifecycle.Now()

	if v.ExpectedState != vm.Stopped {
		v.CurrentState = vm.Starting
		_, err := h.vmRepository.Save(ctx, &v)

		return err
	}

	v.CurrentState = vm.Stopping
	if _, err := h.vmRepository.Save(ctx, &v); err != nil {
		return err
	}

	return h.commander.Stop(ctx, &v)
}
