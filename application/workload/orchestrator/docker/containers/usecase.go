// Package containers answers what the control plane asks of a Docker VM's
// containers, which only the VM's dockerd knows.
package containers

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/internal/reply"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Daemons are the dockerds of this node's Docker VMs.
type Daemons interface {
	Daemon(vmUUID string) docker.Daemon
}

// UseCase answers requests about containers. Each request is one call to the
// VM's dockerd, whose dockerd is answering by the time it is asked: requests
// to a Docker VM wait for that before they are answered.
type UseCase struct {
	daemons Daemons
}

func NewUseCase(daemons Daemons) *UseCase {
	return &UseCase{daemons: daemons}
}

// Answer answers one request, with what the operation answers with and
// whether it had to be cut to fit in a reply.
func (uc *UseCase) Answer(ctx context.Context, request noderequest.Request) (any, bool, error) {
	daemon := uc.daemons.Daemon(request.VMUUID)

	switch request.Op {
	case noderequest.OpContainersList:
		var filter noderequest.ContainersRequest
		if err := reply.Decode(request.Payload, &filter); err != nil {
			return nil, false, err
		}

		listed, err := daemon.Containers(ctx, filter.ToDocker())
		if err != nil {
			return nil, false, err
		}

		containers := make([]noderequest.Container, len(listed))
		for n, c := range listed {
			containers[n] = noderequest.NewContainer(c)
		}

		fitted, truncated := reply.First(containers)

		return fitted, truncated, nil

	case noderequest.OpContainersInspect:
		id, err := containerID(request)
		if err != nil {
			return nil, false, err
		}

		found, err := daemon.Container(ctx, id)
		if err != nil {
			return nil, false, err
		}

		return noderequest.NewContainer(found), false, nil

	case noderequest.OpContainersCreate:
		var spec noderequest.ContainerSpec
		if err := reply.Decode(request.Payload, &spec); err != nil {
			return nil, false, err
		}

		if err := reply.Required("image", spec.Image); err != nil {
			return nil, false, err
		}

		created, err := daemon.CreateContainer(ctx, spec.ToDocker())
		if err != nil {
			return nil, false, err
		}

		return noderequest.NewContainer(created), false, nil

	case noderequest.OpContainersStart, noderequest.OpContainersStop, noderequest.OpContainersRestart:
		id, err := containerID(request)
		if err != nil {
			return nil, false, err
		}

		return nil, false, lifecycle(ctx, daemon, request.Op, id)

	case noderequest.OpContainersRemove:
		var remove noderequest.RemoveRequest
		if err := reply.Decode(request.Payload, &remove); err != nil {
			return nil, false, err
		}

		if err := reply.Required("id", remove.ID); err != nil {
			return nil, false, err
		}

		return nil, false, daemon.RemoveContainer(ctx, remove.ID, remove.Force)

	case noderequest.OpContainersLogs:
		return logs(ctx, daemon, request)

	case noderequest.OpContainersStats:
		id, err := containerID(request)
		if err != nil {
			return nil, false, err
		}

		stats, err := daemon.ContainerStats(ctx, id)
		if err != nil {
			return nil, false, err
		}

		return noderequest.NewStats(stats), false, nil

	case noderequest.OpContainersConnect:
		var connect noderequest.ConnectRequest
		if err := reply.Decode(request.Payload, &connect); err != nil {
			return nil, false, err
		}

		if err := errors.Join(reply.Required("network", connect.Network), reply.Required("container", connect.Container)); err != nil {
			return nil, false, err
		}

		return nil, false, daemon.ConnectNetwork(ctx, connect.Network, connect.Container, connect.Aliases)

	case noderequest.OpContainersDisconnect:
		var disconnect noderequest.DisconnectRequest
		if err := reply.Decode(request.Payload, &disconnect); err != nil {
			return nil, false, err
		}

		if err := errors.Join(reply.Required("network", disconnect.Network), reply.Required("container", disconnect.Container)); err != nil {
			return nil, false, err
		}

		return nil, false, daemon.DisconnectNetwork(ctx, disconnect.Network, disconnect.Container, disconnect.Force)
	}

	return nil, false, reply.Invalid("%q is not an operation on containers", request.Op)
}

func lifecycle(ctx context.Context, daemon docker.Daemon, op noderequest.Op, id string) error {
	switch op {
	case noderequest.OpContainersStart:
		return daemon.StartContainer(ctx, id)
	case noderequest.OpContainersStop:
		return daemon.StopContainer(ctx, id)
	default:
		return daemon.RestartContainer(ctx, id)
	}
}

// logs reads a container's log from its end, cut at the most lines a reply
// carries, and then at what fits in one.
func logs(ctx context.Context, daemon docker.Daemon, request noderequest.Request) (any, bool, error) {
	var asked noderequest.ContainerLogsRequest
	if err := reply.Decode(request.Payload, &asked); err != nil {
		return nil, false, err
	}

	if err := reply.Required("id", asked.ID); err != nil {
		return nil, false, err
	}

	// one line more than a reply carries is how a log that does not fit is
	// told from one that just does.
	options := asked.Options()
	if options.Tail == 0 || options.Tail > noderequest.MaxLogLines {
		options.Tail = noderequest.MaxLogLines + 1
	}

	read, err := daemon.ContainerLogs(ctx, asked.ID, options)
	if err != nil {
		return nil, false, err
	}

	truncated := false
	if len(read) > noderequest.MaxLogLines {
		read = read[len(read)-noderequest.MaxLogLines:]
		truncated = true
	}

	lines := make([]noderequest.LogLine, len(read))
	for n, line := range read {
		lines[n] = noderequest.NewLogLine(line)
	}

	fitted, cut := reply.Last(lines)

	return fitted, truncated || cut, nil
}

func containerID(request noderequest.Request) (string, error) {
	var named noderequest.ContainerRequest
	if err := reply.Decode(request.Payload, &named); err != nil {
		return "", err
	}

	if err := reply.Required("id", named.ID); err != nil {
		return "", err
	}

	return named.ID, nil
}
