package providers

import (
	"context"
	"os"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/pubsub"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/requestreply"
	"github.com/nats-io/nats.go"
)

const (
	BlogRequestReplyer        = "blog::request-replyer"
	BlogRequestReplyerChannel = "blog::request-replyer-channel"

	blogRequestReplyerConsumerID = "blog-request-replyer"
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

	jetstreamPublishSubscriber, err := pubsub.NewPublishSubscriber(natsConnection)
	if err != nil {
		return err
	}

	reqreplyer, replyChan, err := requestreply.New(natsConnection, blogRequestReplyerConsumerID)
	if err != nil {
		return err
	}

	p.terminate = func() {
		defer natsConnection.Drain()
		defer jetstreamPublishSubscriber.Wait()
		defer reqreplyer.Close()
	}

	iocContainer.Singleton(func() *nats.Conn { return natsConnection })
	iocContainer.Singleton(func() domain.Publisher { return jetstreamPublishSubscriber })
	iocContainer.Singleton(func() domain.Subscriber { return jetstreamPublishSubscriber })
	iocContainer.Singleton(func() domain.PublishSubscriber { return jetstreamPublishSubscriber })
	iocContainer.Singleton(func() domain.Requester { return reqreplyer }, ioc.WithNameBinding(BlogRequestReplyer))
	iocContainer.Singleton(func() chan *domain.Reply { return replyChan }, ioc.WithNameBinding(BlogRequestReplyerChannel))

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
