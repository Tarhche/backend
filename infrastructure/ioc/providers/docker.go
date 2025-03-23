package providers

import (
	"context"

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
	containerManager, err := container.NewDockerManager("tcp://docker:2375")
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
