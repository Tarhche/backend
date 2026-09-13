package runner

import (
	"context"

	"github.com/danceable/provider"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

const (
	WorkerName = "runner:worker:name"
)

// workerNameProvider binds the worker name, which the command loads from its
// --name flag or from the RUNNER_WORKER_NAME environment variable, under the
// name the worker providers resolve it by.
type workerNameProvider struct{}

var _ provider.Provider = &workerNameProvider{}

// NewWorkerNameProvider binds the worker name into the container so the worker
// providers can resolve it. It must be registered after the configs provider.
func NewWorkerNameProvider() *workerNameProvider {
	return &workerNameProvider{}
}

func (p *workerNameProvider) Register(ctx context.Context, c provider.Container) error {
	var workerConfigs *configs.RunnerWorker
	if err := c.Resolve(&workerConfigs); err != nil {
		return err
	}

	name := workerConfigs.Name

	return c.Bind(func() string { return name }, provider.Singleton(), provider.WithName(WorkerName))
}

func (p *workerNameProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *workerNameProvider) Terminate(ctx context.Context) error {
	return nil
}
