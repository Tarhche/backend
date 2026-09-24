package providers

import (
	"log/slog"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/initrd"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/launcher"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/layout"
)

// firecrackerRuntime runs an orchestrator's tasks in microVMs, a machine each,
// which the launcher on the same host starts for it. The orchestrator holds no
// privilege for any of it: it decides what a machine is, and asks.
func firecrackerRuntime(orchestratorConfigs *configs.RunnerOrchestrator, logger *slog.Logger) (runtime, error) {
	state := orchestratorConfigs.StateDir

	kernel := orchestratorConfigs.Kernel
	if len(kernel) == 0 {
		kernel = layout.Kernel(state)
	}

	// every machine boots the agent this orchestrator was built with, which
	// is how the agent and what speaks to it never disagree.
	initramfs, err := initrd.Ensure(layout.Boot(state), orchestratorConfigs.GuestBinary)
	if err != nil {
		return runtime{}, err
	}

	tasks, networks, err := firecracker.New(firecracker.Config{
		Owner:       orchestratorConfigs.Name,
		StateDir:    state,
		Kernel:      kernel,
		Initrd:      initramfs,
		Nameservers: orchestratorConfigs.NameserverList(),
		UID:         orchestratorConfigs.OrchestratorUID,
		GID:         orchestratorConfigs.OrchestratorGID,
	}, launcher.NewClient(orchestratorConfigs.LauncherSocket), logger)
	if err != nil {
		return runtime{}, err
	}

	return runtime{
		tasks:    tasks,
		networks: networks,
		node:     firecracker.NewNode(tasks),
		release:  tasks.Close,
	}, nil
}
