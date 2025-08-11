package runTask

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskScheduled struct {
	useCase  *UseCase
	nodeName string
}

func NewTaskScheduled(
	useCase *UseCase,
	nodeName string,
) *TaskScheduled {
	return &TaskScheduled{
		useCase:  useCase,
		nodeName: nodeName,
	}
}

var _ domain.MessageHandler = &TaskScheduled{}

func (uc *TaskScheduled) Handle(data []byte) error {
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
		Image:         taskScheduled.Image,
		AutoRemove:    taskScheduled.AutoRemove,
		PortBindings:  portBindings,
		RestartPolicy: taskScheduled.RestartPolicy,
		HealthCheck:   taskScheduled.HealthCheck,
		RestartCount:  taskScheduled.RestartCount,
		AttachStdin:   taskScheduled.AttachStdin,
		AttachStdout:  taskScheduled.AttachStdout,
		AttachStderr:  taskScheduled.AttachStderr,
		Environment:   taskScheduled.Environment,
		Command:       taskScheduled.Command,
		Entrypoint:    taskScheduled.Entrypoint,
		Mounts:        mounts,
		ResourceLimits: ResourceLimits{
			Cpu:    taskScheduled.ResourceLimits.Cpu,
			Memory: taskScheduled.ResourceLimits.Memory,
			Disk:   taskScheduled.ResourceLimits.Disk,
		},
	}

	_, err := uc.useCase.Execute(request)

	return err
}
