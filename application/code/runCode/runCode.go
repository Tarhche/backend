package runCode

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const (
	RunCodeRequest      = "runCode"
	CodeRunnerOwnerUUID = "guest"

	DefaultMaxDiskSize   = 100 << 20 // 100 MB
	DefaultMaxMemorySize = 200 << 20 // 200 MB
	DefaultMaxCpu        = 2

	// CodeTimeout is how long the code itself is given. The runner image
	// enforces it and says so in the output, which is what somebody running
	// code wants to be told.
	CodeTimeout = 30 * time.Second

	// TTL is how long the container is allowed to exist at all. It is the
	// backstop for a container that ignores the timeout above — the runner
	// takes it away regardless — so it is the longer of the two.
	TTL = 2 * CodeTimeout

	// LiveCodeTimeout is what a snippet gets when there is something to do
	// with it while it runs: a port to open, or a shell to type in. Both are
	// worth more than the half minute it takes to print something, and both
	// end when the container does. It is what the page counts down to, and it
	// is short on purpose: a page anybody can open is a page anybody can leave
	// a container running on.
	LiveCodeTimeout = 2 * time.Minute

	// LiveTTL is the same: what a snippet is given is what its container is
	// allowed, so the countdown a reader watches is the whole of its time. The
	// image's own limit is what usually ends it; the runner takes the
	// container away if it does not.
	LiveTTL = LiveCodeTimeout
)

// codeRetries is how many times a piece of code that could not be run is tried
// again: none. Whatever stopped it — an image that will not pull, a node that
// will not take it — is not something a second attempt fixes, and somebody is
// waiting on the page to be told what happened.
var codeRetries = 0

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

	timeout, ttl := CodeTimeout, TTL
	if request.Live() {
		timeout, ttl = LiveCodeTimeout, LiveTTL
	}

	// a job: it runs once, and what is left of it goes when it ends.
	event := &events.TaskRunRequested{
		Name:       request.ID,
		Kind:       string(task.KindJob),
		Image:      request.Image(),
		TTL:        ttl,
		MaxRetries: &codeRetries,
		Command:    []string{"--timeout", strconv.Itoa(int(timeout.Seconds())), request.Code},
		ResourceLimits: events.ResourceLimits{
			Cpu:    DefaultMaxCpu,
			Memory: DefaultMaxMemorySize,
			Disk:   DefaultMaxDiskSize,
		},
		OwnerUUID: CodeRunnerOwnerUUID,

		// a snippet that serves something is reached by name: the runner
		// publishes these on the node and answers for them at the ingress.
		ExposedPorts: request.Ports,

		// and one somebody is watching is reported as it runs rather than
		// answered once at the end.
		Interactive: request.Live(),
	}

	h.logger.Info("event produced", "event", event)

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return h.producer.Produce(ctx, events.TaskRunRequestedName, payload)
}
