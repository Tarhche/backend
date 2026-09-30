package runner

import (
	"context"

	"github.com/danceable/provider"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

const (
	OrchestratorName = "runner:orchestrator:name"
)

// orchestratorNameProvider binds the orchestrator name, which the command loads from its
// --name flag or from the RUNNER_ORCHESTRATOR_NAME environment variable, under the
// name the orchestrator providers resolve it by.
type orchestratorNameProvider struct{}

var _ provider.Provider = &orchestratorNameProvider{}

// NewOrchestratorNameProvider binds the orchestrator name into the task so the orchestrator
// providers can resolve it. It must be registered after the configs provider.
func NewOrchestratorNameProvider() *orchestratorNameProvider {
	return &orchestratorNameProvider{}
}

func (p *orchestratorNameProvider) Register(ctx context.Context, c provider.Container) error {
	var orchestratorConfigs *configs.RunnerOrchestrator
	if err := c.Resolve(&orchestratorConfigs); err != nil {
		return err
	}

	name := orchestratorConfigs.Name

	return c.Bind(func() string { return name }, provider.Singleton(), provider.WithName(OrchestratorName))
}

func (p *orchestratorNameProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *orchestratorNameProvider) Terminate(ctx context.Context) error {
	return nil
}
