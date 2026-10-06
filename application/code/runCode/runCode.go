package runCode

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

const (
	RunCodeRequest = "runCode"

	// CodeRunnerOwnerUUID is whose the tasks the code runner starts are: the
	// guest's, which is whoever is reading the page.
	CodeRunnerOwnerUUID = task.GuestOwnerUUID

	DefaultMaxDiskSize   = 100 << 20 // 100 MB
	DefaultMaxMemorySize = 200 << 20 // 200 MB
	DefaultMaxCpu        = 2

	// GoMaxMemorySize and GoMaxDiskSize are what a Go snippet is given
	// instead, because it is built before it runs, and its image keeps no
	// build of the standard library: every run compiles from source whatever
	// of it the snippet imports, two packages at a time on its two CPUs.
	//
	// Compiling the runtime package takes more memory than the default: a
	// snippet that prints hello peaks at about 250 MiB in a container, and its
	// compiler is killed in one of 200 MiB, and in a VM of 256 MiB, whose
	// kernel has its share too. A build writes each package twice, where it
	// is built and in the build cache, and the binary besides: one that
	// imports net/http writes 175 MiB at its peak, and fails in the default's
	// 100 MiB. The whole standard library peaks at about 470 MiB of memory
	// and 320 MiB of disk, so a snippet given these may import any of it.
	GoMaxMemorySize = 512 << 20 // 512 MiB
	GoMaxDiskSize   = 512 << 20 // 512 MiB

	// CodeTimeout is how long the code itself is given. The runner image
	// enforces it and says so in the output, which is what somebody running
	// code wants to be told.
	CodeTimeout = 30 * time.Second

	// TTL is how long the task is allowed to exist at all. It is the
	// backstop for a task that ignores the timeout above — the workload
	// takes it away regardless — so it is the longer of the two.
	TTL = 2 * CodeTimeout

	// LiveCodeTimeout is what a snippet gets when there is something to do
	// with it while it runs: a port to open, or a shell to type in. Both are
	// worth more than the half minute it takes to print something, and both
	// end when the task does. It is what the page counts down to, and it
	// is short on purpose: a page anybody can open is a page anybody can leave
	// a task running on.
	LiveCodeTimeout = 2 * time.Minute

	// LiveTTL is the same: what a snippet is given is what its task is
	// allowed, so the countdown a reader watches is the whole of its time. The
	// image's own limit is what usually ends it; the workload takes the
	// task away if it does not.
	LiveTTL = LiveCodeTimeout
)

// codeRetries is how many times a piece of code that could not be run is tried
// again: none. Whatever stopped it — an image that will not pull, a node that
// will not take it — is not something a second attempt fixes, and somebody is
// waiting on the page to be told what happened.
const codeRetries = 0

// Workload is what runs a snippet: the control plane, which admits its task as
// it admits any resource, places it on a node and asks the node to run it.
// What the task does and how it ends reaches the code runner from the nodes
// themselves (see heartbeat).
type Workload interface {
	RunTask(ctx context.Context, ownerUUID string, request workloadControlPlane.TaskRequest) (taskKind.Task, error)
}

// refusal is the error the workload refuses a request with, which says
// field by field what it refused.
type refusal interface {
	error
	Refused() domain.ValidationErrors
}

type runCode struct {
	validator domain.Validator
	workload  Workload
	response  domain.Replyer
	logger    *slog.Logger
}

var _ domain.MessageHandler = &runCode{}

func NewRunCodeHandler(
	validator domain.Validator,
	workload Workload,
	replyer domain.Replyer,
	logger *slog.Logger,
) *runCode {
	return &runCode{
		validator: validator,
		workload:  workload,
		response:  replyer,
		logger:    logger,
	}
}

func (h *runCode) Handle(ctx context.Context, data []byte) error {
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return h.reply(ctx, request.ID, domain.ValidationErrors{
			"runner": "request doesn't have a valid format",
		})
	}

	h.logger.Info("request received", "request", request)

	if validationErrors := h.validator.Validate(&request); len(validationErrors) > 0 {
		h.logger.Warn("validation errors", "validationErrors", validationErrors)

		return h.reply(ctx, request.ID, validationErrors)
	}

	asked := TaskOf(&request)

	_, err := h.workload.RunTask(ctx, CodeRunnerOwnerUUID, asked)

	// a task the workload would not take is answered, rather than asked for
	// again and refused again: what it refused is what the reader is told.
	if refused, ok := errors.AsType[refusal](err); ok && len(refused.Refused()) > 0 {
		h.logger.Warn("the workload refused a snippet's task", "refused", refused.Refused())

		return h.reply(ctx, request.ID, h.validator.Validate(codes(refused.Refused())))
	}

	if err != nil {
		return err
	}

	h.logger.Info("task asked for", "name", asked.Name, "image", asked.Spec.Image)

	return nil
}

// TaskOf is the task a snippet is run in: a job of the guest's, named after
// the request that asked for it, which is what its answers are sent to. It
// runs once, and what is left of it goes when it ends.
func TaskOf(request *Request) workloadControlPlane.TaskRequest {
	timeout, ttl := CodeTimeout, TTL
	if request.Live() {
		timeout, ttl = LiveCodeTimeout, LiveTTL
	}

	retries := codeRetries

	return workloadControlPlane.TaskRequest{
		Name: request.ID,
		Spec: taskKind.Spec{
			Kind:       task.KindJob,
			Image:      request.Image(),
			Command:    []string{"--timeout", strconv.Itoa(int(timeout.Seconds())), request.Code},
			TTL:        ttl,
			Limits:     request.ResourceLimits(),
			MaxRetries: &retries,

			// a snippet that serves something is reached by name: the workload
			// publishes these on the node and answers for them at the ingress.
			Ports: request.Ports,

			// and one somebody is watching is reported as it runs rather than
			// answered once at the end.
			Interactive: request.Live(),
		},
	}
}

func (h *runCode) reply(ctx context.Context, requestID string, validationErrors domain.ValidationErrors) error {
	payload, err := json.Marshal(&Response{ValidationErrors: validationErrors})
	if err != nil {
		return err
	}

	return h.response.Reply(ctx, &domain.Reply{
		RequestID: requestID,
		Payload:   payload,
	})
}

// codes are refusals as codes to put into words, as a request's own are.
type codes domain.ValidationErrors

var _ domain.Validatable = codes{}

func (c codes) Validate() domain.ValidationErrors {
	return domain.ValidationErrors(c)
}
