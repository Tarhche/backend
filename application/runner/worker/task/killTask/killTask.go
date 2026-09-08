package killTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// KillTaskHandler handles the request to kill a task.
type KillTaskHandler struct {
	useCase *UseCase
}

var _ domain.MessageHandler = &KillTaskHandler{}

func NewKillTaskHandler(useCase *UseCase) *KillTaskHandler {
	return &KillTaskHandler{useCase: useCase}
}

func (h *KillTaskHandler) Handle(ctx context.Context, data []byte) error {
	var killRequested events.TaskKillRequested
	if err := json.Unmarshal(data, &killRequested); err != nil {
		return err
	}

	_, err := h.useCase.Execute(ctx, &Request{UUID: killRequested.UUID})
	if err == domain.ErrNotExists {
		// the container is not on this node, or is already gone.
		return nil
	}

	return err
}
