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
	DefaultMaxCpu        = 2
)

type runCode struct {
	validator domain.Validator
	producer  domain.Producer
	response  domain.Replyer
}

var _ domain.MessageHandler = &runCode{}

func NewRunCodeHandler(
	validator domain.Validator,
	producer domain.Producer,
	replyer domain.Replyer,
) *runCode {
	return &runCode{
		validator: validator,
		producer:  producer,
		response:  replyer,
	}
}

func (h *runCode) Handle(data []byte) error {
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		response := &Response{
			ValidationErrors: domain.ValidationErrors{
				"runner": "request doesn't have a valid format",
			},
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return err
		}

		return h.response.Reply(context.Background(), &domain.Reply{
			RequestID: request.ID,
			Payload:   payload,
		})
	}

	log.Printf("request: %+v", request)

	if validationErrors := h.validator.Validate(&request); len(validationErrors) > 0 {
		response := &Response{
			ValidationErrors: validationErrors,
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return err
		}

		log.Printf("validation errors: %+v", validationErrors)

		return h.response.Reply(context.Background(), &domain.Reply{
			RequestID: request.ID,
			Payload:   payload,
		})
	}

	event := &events.TaskRunRequested{
		Name:       request.ID,
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

	return h.producer.Produce(context.Background(), events.TaskRunRequestedName, payload)
}
