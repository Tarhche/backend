package getEndpoint

import (
	"context"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase says where one of this node's containers can be reached.
//
// Only the node holding a container can answer this: the port docker published
// it on is read back off docker every time, because a container that restarted
// came back on a different one.
type UseCase struct {
	containerManager container.Manager

	// advertiseHost is where this node reaches the ports its containers were
	// published on. That is the docker daemon's own host, which is not always
	// this one.
	advertiseHost string
}

func NewUseCase(containerManager container.Manager, advertiseHost string) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		advertiseHost:    advertiseHost,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	containers, err := uc.containerManager.GetByLabel(ctx, container.TaskSlugLabelKey, request.Slug)
	if err != nil {
		return nil, err
	}

	if len(containers) == 0 {
		return nil, ErrNotHeld
	}

	c := containers[len(containers)-1]

	if container.EvaluateTaskState(c.Status, task.Kind(c.Labels[container.TaskKindLabelKey])) != task.Running {
		return nil, ErrNotRunning
	}

	published, found := selectPort(c.PortBindings, request.Port)
	if !found {
		return nil, ErrNotExposed
	}

	return &Response{Host: uc.advertiseHost, Port: published}, nil
}

// selectPort picks the published port a hostname asked for. A hostname naming
// no port reaches the lowest one the container exposes, so the common case of a
// container with a single port needs no port in its name at all.
func selectPort(bindings port.PortMap, requested port.Port) (port.Port, bool) {
	exposed := make([]port.Port, 0, len(bindings))
	for containerPort, published := range bindings {
		if len(published) == 0 || published[0].HostPort == 0 {
			continue
		}

		if requested > 0 && containerPort != requested {
			continue
		}

		exposed = append(exposed, containerPort)
	}

	if len(exposed) == 0 {
		return 0, false
	}

	// docker hands the bindings back in no particular order, and the lowest
	// exposed port is the one a bare hostname reaches.
	lowest := slices.Min(exposed)

	return bindings[lowest][0].HostPort, true
}
