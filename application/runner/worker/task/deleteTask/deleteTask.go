package deleteTask

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const DeleteTaskName = "deleteTask"

// DeleteTaskHandler handles the DeleteTask command
type DeleteTaskHandler struct {
	useCase *UseCase
}

var _ domain.MessageHandler = &DeleteTaskHandler{}

// NewDeleteTaskHandler creates a new DeleteTaskHandler
func NewDeleteTaskHandler(useCase *UseCase) *DeleteTaskHandler {
	return &DeleteTaskHandler{
		useCase: useCase,
	}
}

// Handle handles the DeleteTask command
func (h *DeleteTaskHandler) Handle(data []byte) error {
	var taskDeleted events.TaskDeleted
	if err := json.Unmarshal(data, &taskDeleted); err != nil {
		return err
	}

	request := &Request{UUID: taskDeleted.UUID}

	_, err := h.useCase.Execute(request)
	if err == domain.ErrNotExists {
		return nil
	}

	return err
}
