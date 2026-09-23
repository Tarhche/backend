package providers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/danceable/provider"

	networkContract "github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// runtime is what one runtime binds: the tasks it runs, the networks they
// join, and what the node says about itself. release lets go of whatever the
// runtime holds once the orchestrator is done with it.
type runtime struct {
	tasks    task.Runtime
	networks networkContract.Manager
	node     node.Manager
	release  func(ctx context.Context) error
}

// runtimeProvider binds what runs an orchestrator's tasks. Which runtime that
// is, is the orchestrator's own configuration; nothing it binds says which, so
// nothing that resolves them has to know.
type runtimeProvider struct {
	release func(ctx context.Context) error
}

var _ provider.Provider = &runtimeProvider{}

func NewRuntimeProvider() *runtimeProvider {
	return &runtimeProvider{}
}

func (p *runtimeProvider) Register(ctx context.Context, c provider.Container) error {
	var orchestratorConfigs *configs.RunnerOrchestrator
	if err := c.Resolve(&orchestratorConfigs); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("runner-orchestrator")); err != nil {
		return err
	}

	var (
		selected runtime
		err      error
	)

	switch orchestratorConfigs.Runtime {
	case configs.RuntimeDocker:
		selected, err = dockerRuntime(orchestratorConfigs, logger)
	default:
		return fmt.Errorf("%q is not a runtime; it is either %q or %q", orchestratorConfigs.Runtime, configs.RuntimeFirecracker, configs.RuntimeDocker)
	}

	if err != nil {
		return err
	}

	p.release = selected.release

	if err := c.Bind(func() task.Runtime { return selected.tasks }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() networkContract.Manager { return selected.networks }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() node.Manager { return selected.node }, provider.Singleton())
}

func (p *runtimeProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *runtimeProvider) Terminate(ctx context.Context) error {
	if p.release == nil {
		return nil
	}

	return p.release(ctx)
}
