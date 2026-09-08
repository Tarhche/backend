package beatHeart

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// UseCase reports what this node is running, so the manager can follow every
// container's state and learn the addresses its ports came up on.
type UseCase struct {
	containerManager container.Manager
	messageProducer  domain.Producer
	nodeName         string

	// advertiseHost is the host whose ports the containers on this node are
	// published on. It is what the ingress proxies to, so it has to be an
	// address the manager can reach rather than one this node calls itself.
	advertiseHost string

	logger *slog.Logger
}

func NewUseCase(
	containerManager container.Manager,
	messageProducer domain.Producer,
	nodeName string,
	advertiseHost string,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		messageProducer:  messageProducer,
		nodeName:         nodeName,
		advertiseHost:    advertiseHost,
		logger:           logger,
	}
}

func (uc *UseCase) Execute(ctx context.Context) error {
	allContainers, err := uc.containerManager.GetByLabel(ctx, container.NodeNameLabelKey, uc.nodeName)
	if err != nil {
		return err
	}

	for _, c := range allContainers {
		kind := kindOf(&c)

		event := events.Heartbeat{
			UUID:          c.Labels[container.TaskUUIDLabelKey],
			Name:          c.Labels[container.TaskNameLabelKey],
			Kind:          string(kind),
			Image:         c.Image,
			ContainerUUID: c.ID,
			State:         int(container.EvaluateTaskState(c.Status, kind)),
			NodeName:      uc.nodeName,
			Attempt:       c.Attempt(),
			Endpoints:     uc.endpoints(&c),
			Logs:          uc.logs(ctx, &c),
			At:            time.Now(),
		}

		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}

		if err := uc.messageProducer.Produce(ctx, events.HeartbeatName, payload); err != nil {
			return err
		}
	}

	return nil
}

// kindOf reads what a container is running from the label it was created with.
// A container from before there were kinds is a job, which is what every one of
// them was.
func kindOf(c *container.Container) task.Kind {
	if kind := task.Kind(c.Labels[container.TaskKindLabelKey]); kind.IsValid() {
		return kind
	}

	return task.DefaultKind
}

// logs collects a container's whole output for the heartbeat to carry.
//
// Only a one-shot job's log travels this way: it is what the caller waiting on
// that job receives when it finishes. A long-running service would make every
// heartbeat carry its entire history, so its output is streamed line by line
// and kept by the manager instead.
func (uc *UseCase) logs(ctx context.Context, c *container.Container) []byte {
	if kindOf(c) == task.KindService {
		return nil
	}

	var buffer bytes.Buffer

	if err := uc.containerManager.Logs(ctx, c.ID, &buffer); err != nil {
		// a container that has not started yet has no logs to read, which is
		// ordinary rather than a failure.
		uc.logger.WarnContext(ctx, "failed to fetch container logs", "error", err)

		return nil
	}

	if buffer.Len() == 0 {
		return nil
	}

	return buffer.Bytes()
}

// endpoints reports the addresses a container's exposed ports came up on, so
// the manager can proxy to them. They are read from docker every heartbeat
// because a restarted container comes back on different host ports.
func (uc *UseCase) endpoints(c *container.Container) []events.Endpoint {
	endpoints := make([]events.Endpoint, 0, len(c.PortBindings))

	for containerPort, bindings := range c.PortBindings {
		for _, binding := range bindings {
			if binding.HostPort == 0 {
				continue
			}

			endpoints = append(endpoints, events.Endpoint{
				ContainerPort: containerPort,
				Host:          uc.advertiseHost,
				HostPort:      binding.HostPort,
			})

			break
		}
	}

	// docker hands back the bindings in no particular order, and the lowest
	// exposed port is the one a bare hostname reaches.
	slices.SortFunc(endpoints, func(a events.Endpoint, b events.Endpoint) int {
		return int(a.ContainerPort) - int(b.ContainerPort)
	})

	return endpoints
}
