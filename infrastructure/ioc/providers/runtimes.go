package providers

import (
	"context"
	"log/slog"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	networkContract "github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/container"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microvm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/runtime/multiplex"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/runtime/registry"
)

// runtimesProvider builds a driver for every class this orchestrator offers
// (WORKLOAD_ORCHESTRATOR_RUNTIMES, or sysbox on DOCKER_HOST when it names none)
// and puts one multiplexer in front of them.
//
// The multiplexer is bound as the task.Runtime, network.Manager and
// node.Manager the use cases have always asked for, so none of them knows how
// many classes there are; and as task.Dialer, which reaches a run's port
// through the class holding it. driver.Set is bound for the few that do have to
// know: running a task picks its class's driver, a heartbeat says what each
// class offers, and a stack's network is dropped by the class that made it.
type runtimesProvider struct {
	drivers *registry.Registry
}

var _ provider.Provider = &runtimesProvider{}

func NewRuntimesProvider() *runtimesProvider {
	return &runtimesProvider{}
}

func (p *runtimesProvider) Register(ctx context.Context, c provider.Container) error {
	var orchestratorConfigs *configs.WorkloadOrchestrator
	if err := c.Resolve(&orchestratorConfigs); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("workload-orchestrator")); err != nil {
		return err
	}

	specs, err := orchestratorConfigs.RuntimeSpecs()
	if err != nil {
		return err
	}

	// a kind of driver is a factory here, and nowhere else.
	factories := map[driver.Kind]driver.Factory{
		driver.KindContainer: container.NewFactory(logger),
		driver.KindMicroVM:   microvm.NewFactory(logger),
	}

	drivers, err := registry.New(ctx, orchestratorConfigs.Name, specs, factories, logger)
	if err != nil {
		return err
	}

	p.drivers = drivers

	multiplexer := multiplex.New(drivers, logger)

	if err := c.Bind(func() driver.Set { return multiplexer.Drivers() }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() task.Runtime { return multiplexer }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() task.Dialer { return multiplexer }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() networkContract.Manager { return multiplexer }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() node.Manager { return multiplexer.Node() }, provider.Singleton())
}

func (p *runtimesProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

// Terminate lets go of every driver. What they run carries on: a container is
// its daemon's and a VM its vmhost's, not the orchestrator's.
func (p *runtimesProvider) Terminate(ctx context.Context) error {
	if p.drivers == nil {
		return nil
	}

	return p.drivers.Close()
}
