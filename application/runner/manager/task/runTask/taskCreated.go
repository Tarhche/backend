package runTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const (
	nominatedNodesLimit = 10
)

type TaskCreated struct {
	taskRepository  task.Repository
	nodeRepository  node.Repository
	scheduler       task.Scheduler
	asyncCommandBus domain.PublishSubscriber
}

func NewTaskCreated(
	taskRepository task.Repository,
	nodeRepository node.Repository,
	scheduler task.Scheduler,
	asyncCommandBus domain.PublishSubscriber,
) *TaskCreated {
	return &TaskCreated{
		taskRepository:  taskRepository,
		nodeRepository:  nodeRepository,
		scheduler:       scheduler,
		asyncCommandBus: asyncCommandBus,
	}
}

func (uc *TaskCreated) Handle(data []byte) error {
	var taskCreated events.TaskCreated
	if err := json.Unmarshal(data, &taskCreated); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(taskCreated.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	destinationState := task.Scheduled
	if t.State == destinationState {
		return nil
	}

	if !task.ValidStateTransition(t.State, destinationState) {
		return task.ErrInvalidStateTransition
	}

	nodes, err := uc.nodeRepository.GetAll(0, nominatedNodesLimit)
	if err != nil {
		return err
	}

	if len(nodes) == 0 {
		return node.ErrNoNodesAvailable
	}
	selectedNode := uc.scheduler.Pick(&t, nodes)

	t.State = destinationState
	if _, err = uc.taskRepository.Save(&t); err != nil {
		return err
	}

	return uc.publishTaskScheduled(&t, &selectedNode)
}

func (uc *TaskCreated) publishTaskScheduled(t *task.Task, selectedNode *node.Node) error {
	event := events.TaskScheduled{
		UUID:          t.UUID,
		Name:          t.Name,
		Image:         t.Image,
		AutoRemove:    t.AutoRemove,
		PortBindings:  convertPortBindings(t.PortBindings),
		RestartPolicy: t.RestartPolicy,
		RestartCount:  t.RestartCount,
		HealthCheck:   t.HealthCheck,
		AttachStdin:   t.AttachStdin,
		AttachStdout:  t.AttachStdout,
		AttachStderr:  t.AttachStderr,
		Environment:   t.Environment,
		Command:       t.Command,
		Entrypoint:    t.Entrypoint,
		Mounts:        convertMounts(t.Mounts),
		ResourceLimits: events.ResourceLimits{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		NominatedNode: selectedNode.Name,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Publish(context.Background(), events.TaskScheduledName, payload)
}

func convertPortBindings(domainPorts []port.PortMap) []events.PortMap {
	result := make([]events.PortMap, len(domainPorts))
	for i, p := range domainPorts {
		portMap := make(events.PortMap)
		for portNum, bindings := range p {
			portBindings := make([]events.PortBinding, len(bindings))
			for j, b := range bindings {
				portBindings[j] = events.PortBinding{
					HostIP:   b.HostIP,
					HostPort: b.HostPort,
				}
			}
			portMap[portNum] = portBindings
		}
		result[i] = portMap
	}
	return result
}

func convertMounts(domainMounts []task.Mount) []events.Mount {
	result := make([]events.Mount, len(domainMounts))
	for i, m := range domainMounts {
		result[i] = events.Mount{
			Source:   m.Source,
			Target:   m.Target,
			Type:     m.Type,
			ReadOnly: m.ReadOnly,
		}
	}

	return result
}
