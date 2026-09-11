package providers

import (
	"context"
	"log/slog"

	"github.com/danceable/provider"

	containerContract "github.com/khanzadimahdi/testproject/domain/runner/container"
	networkContract "github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/container"
	infraNetwork "github.com/khanzadimahdi/testproject/infrastructure/runner/network"
	infraNode "github.com/khanzadimahdi/testproject/infrastructure/runner/node"
)

type dockerProvider struct{}

var _ provider.Provider = &dockerProvider{}

func NewDockerProvider() *dockerProvider {
	return &dockerProvider{}
}

func (p *dockerProvider) Register(ctx context.Context, c provider.Container) error {
	var workerConfigs *configs.RunnerWorker
	if err := c.Resolve(&workerConfigs); err != nil {
		return err
	}

	dockerHost := workerConfigs.DockerHost

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("runner-worker")); err != nil {
		return err
	}

	containerManager, err := container.NewDockerManager(dockerHost, logger)
	if err != nil {
		return err
	}

	nodeManager, err := infraNode.NewDockerManager(dockerHost, containerManager)
	if err != nil {
		return err
	}

	networkManager, err := infraNetwork.NewManager(dockerHost, logger)
	if err != nil {
		return err
	}

	if err := c.Bind(func() containerContract.Manager { return containerManager }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() networkContract.Manager { return networkManager }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() node.Manager { return nodeManager }, provider.Singleton())
}

func (p *dockerProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *dockerProvider) Terminate(ctx context.Context) error {
	return nil
}
