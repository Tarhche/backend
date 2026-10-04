package providers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/danceable/provider"

	networkContract "github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/client"
)

// workloadRuntimeProvider binds what runs an orchestrator's tasks:
// task.Runtime, network.Manager and node.Manager, which are all the
// orchestrator's use cases know of it.
//
// WORKLOAD_ORCHESTRATOR_RUNTIME chooses. Sysbox, the default, is exactly what
// the docker provider binds, by way of the docker provider itself, so an
// orchestrator that names no runtime runs as it always has. Microsandbox is the
// workload-microsandbox client. Anything else stops the orchestrator from
// starting, rather than leave it running tasks on something nobody asked for.
type workloadRuntimeProvider struct {
	chosen provider.Provider
}

var _ provider.Provider = &workloadRuntimeProvider{}

func NewWorkloadRuntimeProvider() *workloadRuntimeProvider {
	return &workloadRuntimeProvider{}
}

func (p *workloadRuntimeProvider) Register(ctx context.Context, c provider.Container) error {
	var orchestratorConfigs *configs.WorkloadOrchestrator
	if err := c.Resolve(&orchestratorConfigs); err != nil {
		return err
	}

	switch orchestratorConfigs.Runtime {
	// a configuration made without its defaults names no runtime, and gets
	// the default all the same.
	case configs.RuntimeSysbox, "":
		p.chosen = NewDockerProvider()
	case configs.RuntimeMicrosandbox:
		p.chosen = &microsandboxProvider{}
	default:
		return fmt.Errorf(
			"WORKLOAD_ORCHESTRATOR_RUNTIME is %q, and an orchestrator runs its tasks on %s or %s",
			orchestratorConfigs.Runtime, configs.RuntimeSysbox, configs.RuntimeMicrosandbox,
		)
	}

	return p.chosen.Register(ctx, c)
}

func (p *workloadRuntimeProvider) Boot(ctx context.Context, c provider.Container) error {
	if p.chosen == nil {
		return nil
	}

	return p.chosen.Boot(ctx, c)
}

func (p *workloadRuntimeProvider) Terminate(ctx context.Context) error {
	if p.chosen == nil {
		return nil
	}

	return p.chosen.Terminate(ctx)
}

// microsandboxProvider binds the workload-microsandbox client.
//
// It reaches the service with the certificate the orchestrator already holds
// for the tunnel, under the same authority, so there is no secret of its own to
// hand out. It reaches nothing while it is wired: an orchestrator starts
// whether or not the service is up, as it does whether or not docker is.
type microsandboxProvider struct {
	client *client.Client
}

var _ provider.Provider = &microsandboxProvider{}

func (p *microsandboxProvider) Register(ctx context.Context, c provider.Container) error {
	var orchestratorConfigs *configs.WorkloadOrchestrator
	if err := c.Resolve(&orchestratorConfigs); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("workload-orchestrator")); err != nil {
		return err
	}

	microsandbox, err := client.New(client.Config{
		URL:         orchestratorConfigs.MicrosandboxURL,
		Node:        orchestratorConfigs.Name,
		Authority:   orchestratorConfigs.TunnelAuthority,
		Certificate: orchestratorConfigs.TunnelCertificate,
		PrivateKey:  orchestratorConfigs.TunnelKey,
	}, logger)
	if err != nil {
		return err
	}

	p.client = microsandbox

	taskRuntime := client.NewRuntime(microsandbox)
	networkManager := client.NewNetworkManager()
	nodeManager := client.NewNodeManager(microsandbox)

	if err := c.Bind(func() task.Runtime { return taskRuntime }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() networkContract.Manager { return networkManager }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() node.Manager { return nodeManager }, provider.Singleton())
}

func (p *microsandboxProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

// Terminate lets go of the connections kept to the service. What it runs
// carries on: a run is the service's, not the orchestrator's.
func (p *microsandboxProvider) Terminate(ctx context.Context) error {
	if p.client != nil {
		p.client.Close()
	}

	return nil
}
