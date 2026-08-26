package providers

import (
	"context"

	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

type natsProvider struct {
	terminate func()
}

var _ provider.Provider = &natsProvider{}

func NewNatsProvider() *natsProvider {
	return &natsProvider{}
}

func (p *natsProvider) Register(ctx context.Context, c provider.Container) error {
	var globalConfigs *configs.Global
	if err := c.Resolve(&globalConfigs); err != nil {
		return err
	}

	natsConnection, err := nats.Connect(globalConfigs.Nats.URL)
	if err != nil {
		return err
	}

	p.terminate = func() {
		defer natsConnection.Drain()
	}

	return c.Bind(func() *nats.Conn { return natsConnection }, provider.Singleton())
}

func (p *natsProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *natsProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}
