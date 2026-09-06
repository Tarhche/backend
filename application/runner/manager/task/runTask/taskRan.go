package runTask

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskRan struct {
	taskRepository task.Repository

	// ports is the range the ingress accepts raw connections on. A container
	// that comes up is given one of them for each port it exposes, so that ssh
	// and everything else that is not http has an address of its own. An empty
	// range gives out none.
	ports task.PortRange
}

func NewTaskRan(
	taskRepository task.Repository,
	ports task.PortRange,
) *TaskRan {
	return &TaskRan{
		taskRepository: taskRepository,
		ports:          ports,
	}
}

func (uc *TaskRan) Handle(ctx context.Context, data []byte) error {
	var taskRan events.TaskRan
	if err := json.Unmarshal(data, &taskRan); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(ctx, taskRan.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	endpoints := toEndpoints(taskRan.Endpoints)

	// a container keeps the ports it was given for as long as it is up, so a
	// restart on new host ports does not move the address somebody is using.
	if err := uc.givePublicPorts(ctx, t.Endpoints, endpoints); err != nil {
		return err
	}

	// a running container heartbeats several times a second, and each beat
	// reaches here. Only what has actually changed is worth a write: the
	// endpoints, because a restarted container comes back on new host ports,
	// and the state, the first time it comes up.
	changed := t.NodeName != taskRan.NodeName ||
		t.ContainerID != taskRan.ContainerUUID ||
		!t.Deadline.Equal(taskRan.Deadline) ||
		!slices.Equal(t.Endpoints, endpoints)

	t.NodeName = taskRan.NodeName
	t.ContainerID = taskRan.ContainerUUID
	t.Endpoints = endpoints

	// what the container is running against, which only the node that made it
	// knows: a container that came back is running against a new one.
	t.Deadline = taskRan.Deadline

	if t.CurrentState != task.Running {
		t.CurrentState = task.Running
		t.StartedAt = taskRan.StartedAt
		changed = true
	}

	if !changed {
		return nil
	}

	_, err = uc.taskRepository.Save(ctx, &t)

	return err
}

// givePublicPorts hands each of a container's ports an address of its own on
// the ingress, keeping the ones it already had and taking the rest from what
// no other running container is using.
func (uc *TaskRan) givePublicPorts(ctx context.Context, had []task.Endpoint, endpoints []task.Endpoint) error {
	if uc.ports.Empty() {
		return nil
	}

	kept := make(map[port.Port]port.Port, len(had))
	for _, endpoint := range had {
		if endpoint.PublicPort != 0 {
			kept[endpoint.ContainerPort] = endpoint.PublicPort
		}
	}

	var taken map[port.Port]struct{}

	for i := range endpoints {
		if public, ok := kept[endpoints[i].ContainerPort]; ok {
			endpoints[i].PublicPort = public

			continue
		}

		// what everything else is using is read once, and only when there is
		// something to give out.
		if taken == nil {
			running, err := uc.taskRepository.GetRunningWithPublicPorts(ctx)
			if err != nil {
				return err
			}

			taken = make(map[port.Port]struct{})
			for j := range running {
				for _, endpoint := range running[j].Endpoints {
					if endpoint.PublicPort != 0 {
						taken[endpoint.PublicPort] = struct{}{}
					}
				}
			}
		}

		public, found := uc.ports.Free(taken)
		if !found {
			// nothing left to give: the container is served over http as it
			// always was, and nothing else.
			break
		}

		endpoints[i].PublicPort = public
		taken[public] = struct{}{}
	}

	return nil
}

// toEndpoints reads the addresses a worker published a container on.
func toEndpoints(endpoints []events.Endpoint) []task.Endpoint {
	result := make([]task.Endpoint, len(endpoints))
	for i, e := range endpoints {
		result[i] = task.Endpoint{
			ContainerPort: e.ContainerPort,
			Host:          e.Host,
			HostPort:      e.HostPort,
			HostPortUDP:   e.HostPortUDP,
		}
	}

	return result
}
