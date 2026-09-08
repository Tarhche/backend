package restartTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// RestartTaskHandler handles the request to restart a task.
type RestartTaskHandler struct {
	useCase *UseCase
}

var _ domain.MessageHandler = &RestartTaskHandler{}

func NewRestartTaskHandler(useCase *UseCase) *RestartTaskHandler {
	return &RestartTaskHandler{useCase: useCase}
}

func (h *RestartTaskHandler) Handle(ctx context.Context, data []byte) error {
	var restartRequested events.TaskRestartRequested
	if err := json.Unmarshal(data, &restartRequested); err != nil {
		return err
	}

	_, err := h.useCase.Execute(ctx, &Request{UUID: restartRequested.UUID})
	if err == domain.ErrNotExists {
		// the container is not on this node, or is already gone.
		return nil
	}

	return err
}
