package runCode

import (
	"context"
	"encoding/json"
	"log/slog"

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
	logger    *slog.Logger
}

var _ domain.MessageHandler = &runCode{}

func NewRunCodeHandler(
	validator domain.Validator,
	producer domain.Producer,
	replyer domain.Replyer,
	logger *slog.Logger,
) *runCode {
	return &runCode{
		validator: validator,
		producer:  producer,
		response:  replyer,
		logger:    logger,
	}
}

func (h *runCode) Handle(ctx context.Context, data []byte) error {
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

		return h.response.Reply(ctx, &domain.Reply{
			RequestID: request.ID,
			Payload:   payload,
		})
	}

	h.logger.Info("request received", "request", request)

	if validationErrors := h.validator.Validate(&request); len(validationErrors) > 0 {
		response := &Response{
			ValidationErrors: validationErrors,
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return err
		}

		h.logger.Warn("validation errors", "validationErrors", validationErrors)

		return h.response.Reply(ctx, &domain.Reply{
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

	h.logger.Info("event produced", "event", event)

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return h.producer.Produce(ctx, events.TaskRunRequestedName, payload)
}
