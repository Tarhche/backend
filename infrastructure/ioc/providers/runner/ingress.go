package runner

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	taskrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
)

type ingressProvider struct{}

var _ provider.Provider = &ingressProvider{}

// NewIngressProvider wires what serves the containers themselves: the handler
// that routes a request to a container by the hostname it was made to, and the
// forwarder that carries whole connections to the ports it was given.
//
// It reads the tasks straight from the database rather than asking the manager
// for them, which is the point of it being a service of its own: what a reader
// is looking at does not stop being served because the manager is restarting.
func NewIngressProvider() *ingressProvider {
	return &ingressProvider{}
}

func (p *ingressProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *ingressProvider) Boot(ctx context.Context, c provider.Container) error {
	var database *mongo.Database
	if err := c.Resolve(&database); err != nil {
		return err
	}

	var ingressConfigs *configs.RunnerIngress
	if err := c.Resolve(&ingressConfigs); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("runner-ingress")); err != nil {
		return err
	}

	tasks := taskrepository.NewRepository(database)

	if err := c.Bind(func() http.Handler {
		return ingress.NewHandler(tasks, ingressConfigs.Domain)
	}, provider.Singleton()); err != nil {
		return err
	}

	forwarder, err := ingress.NewForwarder(tasks, ingressConfigs.PortRange, logger)
	if err != nil {
		return err
	}

	return c.Bind(func() *ingress.Forwarder { return forwarder }, provider.Singleton())
}

func (p *ingressProvider) Terminate(ctx context.Context) error {
	return nil
}
