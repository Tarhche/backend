package providers

import (
	"os"

	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/nats-io/nats.go"
)

type natsProvider struct {
	terminate func()
}

var _ ioc.ServiceProvider = &natsProvider{}

func NewNatsProvider() *natsProvider {
	return &natsProvider{}
}

func (p *natsProvider) Register(app *ioc.Application) error {
	natsConnection, err := nats.Connect(os.Getenv("NATS_URL"))
	if err != nil {
		return err
	}

	app.Container.Singleton(func() *nats.Conn { return natsConnection })

	p.terminate = func() {
		defer natsConnection.Drain()
	}

	return nil
}

func (p *natsProvider) Boot(app *ioc.Application) error {
	return nil
}

func (p *natsProvider) Terminate() error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}
