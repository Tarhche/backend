package providers

import (
	"context"
	"os"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	containerContract "github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/container"
	infraNode "github.com/khanzadimahdi/testproject/infrastructure/runner/node"
)

type dockerProvider struct{}

var _ provider.Provider = &dockerProvider{}

func NewDockerProvider() *dockerProvider {
	return &dockerProvider{}
}

func (p *dockerProvider) Register(ctx context.Context, c provider.Container) error {
	dockerHost := os.Getenv("DOCKER_HOST")

	containerManager, err := container.NewDockerManager(dockerHost)
	if err != nil {
		return err
	}

	nodeManager, err := infraNode.NewDockerManager(dockerHost, containerManager)
	if err != nil {
		return err
	}

	if err := c.Bind(func() containerContract.Manager { return containerManager }, bind.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() node.Manager { return nodeManager }, bind.Singleton())
}

func (p *dockerProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *dockerProvider) Terminate(ctx context.Context) error {
	return nil
}
