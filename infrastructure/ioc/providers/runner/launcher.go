//go:build linux

package runner

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danceable/provider"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	"github.com/khanzadimahdi/testproject/application/runner/launcher/ensureNetwork"
	"github.com/khanzadimahdi/testproject/application/runner/launcher/getMachines"
	"github.com/khanzadimahdi/testproject/application/runner/launcher/launchMachine"
	"github.com/khanzadimahdi/testproject/application/runner/launcher/removeNetwork"
	"github.com/khanzadimahdi/testproject/application/runner/launcher/terminateMachine"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/hostnet"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/layout"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/vmm"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	launcherAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/launcher/api"
)

const (
	// LauncherHandler is the launcher's orders, served on its socket, and
	// LauncherHealth its healthcheck, served on loopback.
	LauncherHandler = "runner:launcher:handler"
	LauncherHealth  = "runner:launcher:health"
)

// launcherProvider builds the launcher: what starts machines' processes, what
// plugs them in, and the handler that takes orders for both.
type launcherProvider struct{}

var _ provider.Provider = &launcherProvider{}

func NewLauncherProvider() *launcherProvider {
	return &launcherProvider{}
}

func (p *launcherProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *launcherProvider) Boot(ctx context.Context, c provider.Container) error {
	var launcherConfigs *configs.RunnerLauncher
	if err := c.Resolve(&launcherConfigs); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("runner-launcher")); err != nil {
		return err
	}

	var validator domain.Validator
	if err := c.Resolve(&validator); err != nil {
		return err
	}

	pool, err := launcherConfigs.Pool()
	if err != nil {
		return err
	}

	// what the orchestrators write is theirs, and where machines are kept is
	// the launcher's; and the kernel is where machines boot it from.
	if err := layout.Prepare(launcherConfigs.StateDir, launcherConfigs.Kernel, launcherConfigs.OrchestratorUID, launcherConfigs.OrchestratorGID); err != nil {
		return err
	}

	machines, err := vmm.New(vmm.Config{
		StateDir:          launcherConfigs.StateDir,
		FirecrackerBinary: launcherConfigs.FirecrackerBinary,
		FirstUID:          launcherConfigs.MachineFirstUID,
		UIDs:              launcherConfigs.MachineUIDs,
		ClientGID:         launcherConfigs.OrchestratorGID,
	}, logger)
	if err != nil {
		return err
	}

	network, err := hostnet.New(hostnet.Config{
		Pool:     pool,
		FirstUID: launcherConfigs.MachineFirstUID,
		UIDs:     launcherConfigs.MachineUIDs,
	}, logger)
	if err != nil {
		return err
	}

	limits := launchMachine.Limits{VCPUs: launcherConfigs.MaxVCPUs, MemoryMiB: launcherConfigs.MaxMemoryMiB}

	mux := http.NewServeMux()
	mux.Handle("POST /machines", launcherAPI.NewLaunchHandler(launchMachine.NewUseCase(machines, network, validator, limits)))
	mux.Handle("GET /machines", launcherAPI.NewIndexHandler(getMachines.NewUseCase(machines, validator)))
	mux.Handle("DELETE /machines/{id}", launcherAPI.NewTerminateHandler(terminateMachine.NewUseCase(machines, network, validator)))
	mux.Handle("PUT /networks/{owner}/{name}", launcherAPI.NewEnsureNetworkHandler(ensureNetwork.NewUseCase(network, validator)))
	mux.Handle("DELETE /networks/{owner}/{name}", launcherAPI.NewRemoveNetworkHandler(removeNetwork.NewUseCase(network, validator)))

	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/runner/launcher",
				middleware.NewLogMiddleware(mux, logger),
			),
		),
		logger,
	)

	// the launcher depends on nothing it could ping: being up is being able
	// to answer.
	health := healthAPI.NewHealthHandler(checkhealth.NewUseCase())

	if err := c.Bind(func() http.Handler { return handler }, provider.Singleton(), provider.WithName(LauncherHandler)); err != nil {
		return err
	}

	return c.Bind(func() http.Handler { return health }, provider.Singleton(), provider.WithName(LauncherHealth))
}

func (p *launcherProvider) Terminate(ctx context.Context) error {
	return nil
}
