package providers

import (
	"context"
	"os"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream"
	"github.com/nats-io/nats.go"
)

type natsProvider struct {
	terminate func()
}

var _ ioc.ServiceProvider = &natsProvider{}

func NewNatsProvider() *natsProvider {
	return &natsProvider{}
}

func (p *natsProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	natsConnection, err := nats.Connect(os.Getenv("NATS_URL"))
	if err != nil {
		return err
	}

	jetstreamPublishSubscriber, err := jetstream.NewPublishSubscriber(natsConnection)
	if err != nil {
		return err
	}

	p.terminate = func() {
		defer jetstreamPublishSubscriber.Wait()
		defer natsConnection.Close()
	}

	iocContainer.Singleton(func() *nats.Conn { return natsConnection })
	iocContainer.Singleton(func() domain.PublishSubscriber { return jetstreamPublishSubscriber })

	return nil
}

func (p *natsProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *natsProvider) Terminate() error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}
