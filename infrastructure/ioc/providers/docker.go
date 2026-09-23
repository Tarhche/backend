package providers

import (
	"context"
	"log/slog"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	infraContainer "github.com/khanzadimahdi/testproject/infrastructure/runner/docker/container"
	infraNetwork "github.com/khanzadimahdi/testproject/infrastructure/runner/docker/network"
	infraNode "github.com/khanzadimahdi/testproject/infrastructure/runner/docker/node"
)

// dockerRuntime runs an orchestrator's tasks as containers on a docker daemon,
// which need not be on this machine.
func dockerRuntime(orchestratorConfigs *configs.RunnerOrchestrator, logger *slog.Logger) (runtime, error) {
	dockerHost := orchestratorConfigs.DockerHost

	taskRuntime, err := infraContainer.NewDockerManager(dockerHost, orchestratorConfigs.DockerAdvertiseHost, logger)
	if err != nil {
		return runtime{}, err
	}

	nodeManager, err := infraNode.NewDockerManager(dockerHost, taskRuntime)
	if err != nil {
		return runtime{}, err
	}

	networkManager, err := infraNetwork.NewManager(dockerHost, logger)
	if err != nil {
		return runtime{}, err
	}

	return runtime{
		tasks:    taskRuntime,
		networks: networkManager,
		node:     nodeManager,
		release:  func(context.Context) error { return nil },
	}, nil
}
