package snapshotVM

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
)

// SnapshotRequestedHandler takes the snapshots the control plane asks this
// node for.
type SnapshotRequestedHandler struct {
	useCase  *UseCase
	nodeName string
	logger   *slog.Logger
}

var _ domain.MessageHandler = &SnapshotRequestedHandler{}

func NewSnapshotRequestedHandler(useCase *UseCase, nodeName string, logger *slog.Logger) *SnapshotRequestedHandler {
	return &SnapshotRequestedHandler{useCase: useCase, nodeName: nodeName, logger: logger}
}

func (h *SnapshotRequestedHandler) Handle(ctx context.Context, data []byte) error {
	var requested events.SnapshotRequested
	if err := json.Unmarshal(data, &requested); err != nil {
		h.logger.ErrorContext(ctx, "a snapshot could not be taken from an unreadable message", "error", err)

		return nil
	}

	if requested.NodeName != h.nodeName {
		return nil
	}

	request := &Request{SnapshotUUID: requested.SnapshotUUID, VMUUID: requested.VMUUID}

	response, err := h.useCase.Execute(ctx, request)
	if err != nil {
		return err
	}

	// a refused command is a snapshot that will never be made, which is said
	// against the snapshot rather than retried.
	if len(response.ValidationErrors) == 0 || len(requested.SnapshotUUID) == 0 {
		return nil
	}

	fields := slices.Sorted(maps.Keys(response.ValidationErrors))

	reasons := make([]string, len(fields))
	for n, field := range fields {
		reasons[n] = field + ": " + response.ValidationErrors[field]
	}

	return h.useCase.failed(ctx, requested.SnapshotUUID, fmt.Errorf("the command was refused: %s", strings.Join(reasons, "; ")))
}
