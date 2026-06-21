package providers

import (
	"context"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"
)

// containerProvider exposes the dependency injection container itself so that
// handler builders can perform named bindings and resolutions.
type containerProvider struct{}

var _ provider.Provider = &containerProvider{}

func NewContainerProvider() *containerProvider {
	return &containerProvider{}
}

func (p *containerProvider) Register(ctx context.Context, c provider.Container) error {
	return c.Bind(func() provider.Container { return c }, bind.Singleton())
}

func (p *containerProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *containerProvider) Terminate(ctx context.Context) error {
	return nil
}
