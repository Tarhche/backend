package providers

import (
	"context"

	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
)

type containerProvider struct{}

var _ ioc.ServiceProvider = &containerProvider{}

func NewContainerProvider() *containerProvider {
	return &containerProvider{}
}

func (p *containerProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return iocContainer.Singleton(func() ioc.ServiceContainer { return iocContainer })
}

func (p *containerProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *containerProvider) Terminate() error {
	return nil
}
