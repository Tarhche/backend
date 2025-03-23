package runCode

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const (
	RunCodeRequest      = "runCodeRequest"
	CodeRunnerOwnerUUID = "blog:codeRunner"
)

type runCode struct {
	validator domain.Validator
	publisher domain.Publisher
}

var _ domain.Replyer = &runCode{}

func NewRunCodeHandler(
	validator domain.Validator,
	publisher domain.Publisher,
) *runCode {
	return &runCode{
		validator: validator,
		publisher: publisher,
	}
}

func (h *runCode) Reply(r domain.Request, replyChan chan<- *domain.Reply) error {
	var request Request
	if err := json.Unmarshal(r.Payload, &request); err != nil {
		return err
	}

	if validationErrors := h.validator.Validate(request); len(validationErrors) > 0 {
		payload, err := json.Marshal(validationErrors)
		if err != nil {
			return err
		}

		replyChan <- &domain.Reply{
			RequestID: r.ID,
			Payload:   payload,
		}

		return nil
	}

	event := &events.TaskRunRequested{
		Name:       r.ID,
		Image:      request.Image(),
		AutoRemove: true,
		Command:    []string{request.Code},
		OwnerUUID:  CodeRunnerOwnerUUID,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return h.publisher.Publish(context.Background(), events.TaskRunRequestedName, payload)
}
