package providers

import (
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
)

type containerProvider struct{}

var _ ioc.ServiceProvider = &containerProvider{}

func NewContainerProvider() *containerProvider {
	return &containerProvider{}
}

func (p *containerProvider) Register(app *ioc.Application) error {
	return app.Container.Singleton(func() ioc.ServiceContainer { return app.Container })
}

func (p *containerProvider) Boot(app *ioc.Application) error {
	return nil
}

func (p *containerProvider) Terminate() error {
	return nil
}
