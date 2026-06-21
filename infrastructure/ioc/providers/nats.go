package providers

import (
	"context"
	"os"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"
)

type natsProvider struct {
	terminate func()
}

var _ provider.Provider = &natsProvider{}

func NewNatsProvider() *natsProvider {
	return &natsProvider{}
}

func (p *natsProvider) Register(ctx context.Context, c provider.Container) error {
	natsConnection, err := nats.Connect(os.Getenv("NATS_URL"))
	if err != nil {
		return err
	}

	p.terminate = func() {
		defer natsConnection.Drain()
	}

	return c.Bind(func() *nats.Conn { return natsConnection }, bind.Singleton())
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
