package container

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/khanzadimahdi/testproject/domain/runner"
)

type manager struct {
	cli *client.Client
}

var _ runner.ContainerManager = &manager{}

func NewManager(cli *client.Client) *manager {
	return &manager{
		cli: cli,
	}
}

// CreateContainer creates a new container and returns it ID.
func (m *manager) CreateContainer(ctx context.Context, task runner.Task) (string, error) {
	config := &container.Config{
		Image: task.Image,
		Cmd:   task.Command,
	}

	res, err := m.cli.ContainerCreate(ctx, config, &container.HostConfig{}, nil, nil, task.Name)
	if err != nil {
		return "", err
	}

	return res.ID, nil
}

// StartContainer starts the container created with given ID.
func (m *manager) StartContainer(ctx context.Context, id string) error {
	return m.cli.ContainerStart(ctx, id, container.StartOptions{})
}

// WaitForContainer waits for the running container to finish.
func (m *manager) WaitForContainer(ctx context.Context, id string) (bool, error) {
	// check if the container is in running state
	if _, err := m.cli.ContainerInspect(ctx, id); err != nil {
		return true, nil
	}

	// send API call to wait for the container completion
	wait, errC := m.cli.ContainerWait(ctx, id, container.WaitConditionNotRunning)

	// check if container exit code is 0, and return accordingly
	select {
	case status := <-wait:
		if status.StatusCode == 0 {
			return true, nil
		}

		return false, nil
	case err := <-errC:
		return false, err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// RemoveContainer removes the given container id
func (m *manager) RemoveContainer(ctx context.Context, id string) error {
	return m.cli.ContainerRemove(ctx, id, container.RemoveOptions{})
}
