package stopTask

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const StopTaskName = "stopTask"

// StoppageTaskHandler handles the StopTask command
type StoppageTaskHandler struct {
	useCase *UseCase
}

var _ domain.MessageHandler = &StoppageTaskHandler{}

// NewStoppageTaskHandler creates a new StoppageTaskHandler
func NewStoppageTaskHandler(useCase *UseCase) *StoppageTaskHandler {
	return &StoppageTaskHandler{
		useCase: useCase,
	}
}

// Handle handles the StopTask command
func (h *StoppageTaskHandler) Handle(data []byte) error {
	var taskStoppageRequested events.TaskStoppageRequested
	if err := json.Unmarshal(data, &taskStoppageRequested); err != nil {
		return err
	}

	request := &Request{UUID: taskStoppageRequested.UUID}

	_, err := h.useCase.Execute(request)
	if err == domain.ErrNotExists {
		return nil
	}

	return err
}
