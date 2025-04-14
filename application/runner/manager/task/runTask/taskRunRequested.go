package runTask

import (
	"encoding/json"
	"log"

	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskRunRequested struct {
	usecase *UseCase
}

func NewTaskRunRequested(
	usecase *UseCase,
) *TaskRunRequested {
	return &TaskRunRequested{
		usecase: usecase,
	}
}

func (uc *TaskRunRequested) Handle(data []byte) error {
	log.Println("task run requested event received", string(data))

	var event events.TaskRunRequested
	if err := json.Unmarshal(data, &event); err != nil {
		log.Println("error unmarshalling request", err)

		return nil
	}

	portBindings := make(map[uint][]PortBinding, len(event.PortBindings))
	for hostPort, containerPorts := range event.PortBindings {
		portBindings[hostPort] = make([]PortBinding, len(containerPorts))
		for i, containerPort := range containerPorts {
			portBindings[hostPort][i] = PortBinding{
				HostIP:   containerPort.HostIP,
				HostPort: uint(containerPort.HostPort),
			}
		}
	}

	mounts := make([]Mount, len(event.Mounts))
	for i, mount := range event.Mounts {
		mounts[i] = Mount{
			Source:   mount.Source,
			Target:   mount.Target,
			Type:     mount.Type,
			ReadOnly: mount.ReadOnly,
		}
	}

	resourceLimits := ResourceLimits{
		Cpu:    event.ResourceLimits.Cpu,
		Memory: event.ResourceLimits.Memory,
		Disk:   event.ResourceLimits.Disk,
	}

	request := &Request{
		Name:           event.Name,
		Image:          event.Image,
		AutoRemove:     event.AutoRemove,
		PortBindings:   portBindings,
		RestartPolicy:  event.RestartPolicy,
		RestartCount:   event.RestartCount,
		HealthCheck:    event.HealthCheck,
		AttachStdin:    event.AttachStdin,
		AttachStdout:   event.AttachStdout,
		AttachStderr:   event.AttachStderr,
		Environment:    event.Environment,
		Command:        event.Command,
		Entrypoint:     event.Entrypoint,
		Mounts:         mounts,
		ResourceLimits: resourceLimits,
		OwnerUUID:      event.OwnerUUID,
	}

	// TODO: using usecase in handler ? (is this a good idea?)
	response, err := uc.usecase.Execute(request)
	if len(response.ValidationErrors) > 0 {
		log.Println("validation errors", response.ValidationErrors)
	}

	if err != nil {
		log.Println("error running task", err)
	}

	return err
}
