// Package failVM hears from nodes about VMs they could not make what they
// were asked to.
package failVM

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

const (
	// skew is how much earlier than the latest request a failure may say it
	// happened and still be taken for that request's. The node's clock and the
	// control plane's are not the same clock, and a request that fails at once
	// is reported within a moment of being made.
	skew = 5 * time.Second

	// defaultReason is said of a VM whose node did not say what went wrong.
	defaultReason = "the vm failed"
)

// VMFailed writes down that a VM could not be made what it was asked to be,
// and gives up on what was asked.
//
// A command that failed fails the same way when it is sent again — an image
// that is not there, a node with no room, an archive from another engine — so
// a VM that was wanted running is not asked for again and again: what it is,
// is now also what is wanted of it, until somebody asks for something else. A
// VM that fails while its node goes on reporting it is a different thing, and
// is brought back by the control plane's heartbeat.
type VMFailed struct {
	vmRepository vm.Repository
	logger       *slog.Logger
}

var _ domain.MessageHandler = &VMFailed{}

func NewVMFailed(vmRepository vm.Repository, logger *slog.Logger) *VMFailed {
	return &VMFailed{vmRepository: vmRepository, logger: logger}
}

// Handle never fails for a message that will never be handled: redelivering it
// would only fail the same way, at once and for ever.
func (h *VMFailed) Handle(ctx context.Context, data []byte) error {
	var failed events.VMFailed
	if err := json.Unmarshal(data, &failed); err != nil {
		h.logger.ErrorContext(ctx, "a vm failure that cannot be read", "error", err)

		return nil
	}

	v, err := h.vmRepository.GetOne(ctx, failed.VMUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	h.logger.WarnContext(ctx, "a vm failed", "uuid", v.UUID, "node", failed.NodeName, "state", v.CurrentState.String(), "reason", failed.Reason)

	// a VM on its way out stays on its way out; the heartbeat asks its node
	// again.
	if v.CurrentState == vm.Deleting {
		return nil
	}

	// a failure of what was asked before the latest request is that request's
	// no longer.
	if !failed.At.IsZero() && failed.At.Before(v.UpdatedAt.Add(-skew)) {
		return nil
	}

	v.CurrentState = vm.Failed
	v.Reason = failed.Reason
	if len(v.Reason) == 0 {
		v.Reason = defaultReason
	}

	// only what was wanted running is given up on: a VM that failed on its
	// way to being stopped is not running either, which is what was asked.
	if v.ExpectedState == vm.Running {
		v.ExpectedState = vm.Failed
	}

	_, err = h.vmRepository.Save(ctx, &v)

	return err
}
