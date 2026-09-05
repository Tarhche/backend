package runTask

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskScheduled struct {
	useCase  *UseCase
	producer domain.Producer
	nodeName string
	logger   *slog.Logger
}

func NewTaskScheduled(
	useCase *UseCase,
	producer domain.Producer,
	nodeName string,
	logger *slog.Logger,
) *TaskScheduled {
	return &TaskScheduled{
		useCase:  useCase,
		producer: producer,
		nodeName: nodeName,
		logger:   logger,
	}
}

var _ domain.MessageHandler = &TaskScheduled{}

func (uc *TaskScheduled) Handle(ctx context.Context, data []byte) error {
	var taskScheduled events.TaskScheduled
	if err := json.Unmarshal(data, &taskScheduled); err != nil {
		return err
	}

	// skip the tasks that should be scheduled on another node
	if uc.nodeName != taskScheduled.NominatedNode {
		return nil
	}

	// Convert port bindings
	portBindings := make(map[uint][]PortBinding, len(taskScheduled.PortBindings))
	for _, pm := range taskScheduled.PortBindings {
		for port, bindings := range pm {
			pbList := make([]PortBinding, len(bindings))
			for i, b := range bindings {
				pbList[i] = PortBinding{
					HostIP:   b.HostIP,
					HostPort: uint(b.HostPort),
				}
			}
			portBindings[uint(port)] = pbList
		}
	}

	// Convert mounts
	mounts := make([]Mount, len(taskScheduled.Mounts))
	for i, m := range taskScheduled.Mounts {
		mounts[i] = Mount{
			Source:   m.Source,
			Target:   m.Target,
			Type:     m.Type,
			ReadOnly: m.ReadOnly,
		}
	}

	request := &Request{
		UUID:          taskScheduled.UUID,
		Name:          taskScheduled.Name,
		Slug:          taskScheduled.Slug,
		Kind:          task.Kind(taskScheduled.Kind),
		StackUUID:     taskScheduled.StackUUID,
		StackSlug:     taskScheduled.StackSlug,
		ServiceName:   taskScheduled.ServiceName,
		Image:         taskScheduled.Image,
		AutoRemove:    taskScheduled.AutoRemove,
		PortBindings:  portBindings,
		ExposedPorts:  taskScheduled.ExposedPorts,
		NetworkPolicy: taskScheduled.NetworkPolicy,
		RestartPolicy: taskScheduled.RestartPolicy,
		HealthCheck:   taskScheduled.HealthCheck,
		RestartCount:  taskScheduled.RestartCount,
		AttachStdin:   taskScheduled.AttachStdin,
		AttachStdout:  taskScheduled.AttachStdout,
		AttachStderr:  taskScheduled.AttachStderr,
		Environment:   taskScheduled.Environment,
		Command:       taskScheduled.Command,
		Entrypoint:    taskScheduled.Entrypoint,
		WorkingDir:    taskScheduled.WorkingDir,
		ReadOnly:      taskScheduled.ReadOnly,
		Interactive:   taskScheduled.Interactive,
		TTL:           taskScheduled.TTL,
		Mounts:        mounts,
		ResourceLimits: ResourceLimits{
			Cpu:    taskScheduled.ResourceLimits.Cpu,
			Memory: taskScheduled.ResourceLimits.Memory,
			Disk:   taskScheduled.ResourceLimits.Disk,
		},
		Attempt:    taskScheduled.Attempt,
		MaxRetries: taskScheduled.MaxRetries,
	}

	if _, err := uc.useCase.Execute(ctx, request); err != nil {
		// there is no container, so nothing will ever report what became of
		// this one: saying so here is what keeps it from sitting in the state
		// it was scheduled in for good, with the node trying again and again
		// behind everybody's back.
		return uc.reportFailure(ctx, &taskScheduled, err)
	}

	return nil
}

// reportFailure says that a task could not be started, and why.
//
// The failure is announced rather than returned, because returning it asks for
// the message to be delivered again: an image that cannot be pulled will not
// pull on the next attempt either, and the person waiting deserves to be told
// rather than left watching a task that never moves.
func (uc *TaskScheduled) reportFailure(ctx context.Context, scheduled *events.TaskScheduled, cause error) error {
	uc.logger.ErrorContext(ctx, "could not start a task",
		"error", cause, "uuid", scheduled.UUID, "image", scheduled.Image, "attempt", scheduled.Attempt)

	event := events.TaskFailed{
		UUID:       scheduled.UUID,
		Name:       scheduled.Name,
		NodeName:   uc.nodeName,
		At:         time.Now(),
		Attempt:    scheduled.Attempt,
		MaxRetries: scheduled.MaxRetries,
		Reason:     cause.Error(),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.TaskFailedName, payload)
}
