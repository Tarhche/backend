//go:build microsandbox

package workload

import (
	"context"
	"log/slog"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/sdk"
)

// microsandboxSDKProvider binds the run supervisor's port, runs.Sandboxes, to
// microsandbox's Go SDK. The SDK is cgo, so this is compiled only with the
// microsandbox tag, into the workload-microsandbox image.
//
// It resolves the workload-microsandbox logger, so it is registered after the
// OpenTelemetry provider, and before whatever resolves runs.Sandboxes.
// Microsandbox itself is configured by its own environment variables, which
// the SDK reads: MSB_HOME, MSB_PATH and MSB_LIBKRUNFW_PATH.
type microsandboxSDKProvider struct{}

var _ provider.Provider = &microsandboxSDKProvider{}

func NewMicrosandboxSDKProvider() *microsandboxSDKProvider {
	return &microsandboxSDKProvider{}
}

func (p *microsandboxSDKProvider) Register(ctx context.Context, c provider.Container) error {
	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("workload-microsandbox")); err != nil {
		return err
	}

	sandboxes := sdk.New(logger)

	return c.Bind(func() runs.Sandboxes { return sandboxes }, provider.Singleton())
}

func (p *microsandboxSDKProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *microsandboxSDKProvider) Terminate(ctx context.Context) error {
	return nil
}
