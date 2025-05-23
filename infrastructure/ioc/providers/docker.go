package providers

import (
	"context"
	"os"

	containerContract "github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/container"
)

type dockerProvider struct{}

var _ ioc.ServiceProvider = &dockerProvider{}

func NewDockerProvider() *dockerProvider {
	return &dockerProvider{}
}

func (p *dockerProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	dockerHost := os.Getenv("DOCKER_HOST")

	containerManager, err := container.NewDockerManager(dockerHost)
	if err != nil {
		return err
	}

	return iocContainer.Singleton(func() containerContract.Manager { return containerManager })
}

func (p *dockerProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *dockerProvider) Terminate() error {
	return nil
}
