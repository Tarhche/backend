package runCode

import (
	"context"
	"encoding/json"
	"log"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const (
	RunCodeRequest      = "runCode"
	CodeRunnerOwnerUUID = "guest"

	DefaultMaxDiskSize   = 100 << 20 // 100 MB
	DefaultMaxMemorySize = 200 << 20 // 200 MB
	DefaultMaxCpu        = 1
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

	log.Printf("request: %+v", request)

	if validationErrors := h.validator.Validate(&request); len(validationErrors) > 0 {
		response := Response{
			ValidationErrors: validationErrors,
		}

		payload, err := json.Marshal(&response)
		if err != nil {
			return err
		}

		log.Printf("validation errors: %+v", validationErrors)

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
		Command:    []string{"--timeout", "30", request.Code},
		ResourceLimits: events.ResourceLimits{
			Cpu:    DefaultMaxCpu,
			Memory: DefaultMaxMemorySize,
			Disk:   DefaultMaxDiskSize,
		},
		OwnerUUID: CodeRunnerOwnerUUID,
	}

	log.Printf("event: %+v", event)

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return h.publisher.Publish(context.Background(), events.TaskRunRequestedName, payload)
}
