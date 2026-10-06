package runCommand

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// CommandHandler carries out the commands addressed to this node, of every
// kind it runs.
//
// Every node hears every command and carries out only those addressed to it,
// as it does the commands about VMs, snapshots and stacks.
type CommandHandler struct {
	useCase  *UseCase
	nodeName string
	logger   *slog.Logger
}

var _ domain.MessageHandler = &CommandHandler{}

func NewCommandHandler(useCase *UseCase, nodeName string, logger *slog.Logger) *CommandHandler {
	return &CommandHandler{useCase: useCase, nodeName: nodeName, logger: logger}
}

func (h *CommandHandler) Handle(ctx context.Context, data []byte) error {
	var command kind.Command
	if err := json.Unmarshal(data, &command); err != nil {
		// read again, it is as unreadable, and it says nothing a result could
		// be sent back about.
		h.logger.ErrorContext(ctx, "a command could not be carried out from an unreadable message", "error", err)

		return nil
	}

	if command.Node != h.nodeName {
		return nil
	}

	return h.useCase.Execute(ctx, command)
}
