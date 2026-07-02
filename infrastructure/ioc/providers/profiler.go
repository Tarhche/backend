package providers

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/danceable/container/bind"
	"github.com/danceable/container/resolve"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
)

// terminateTimeout bounds the final flush of queued profiles on shutdown.
const profilerTerminateTimeout = 10 * time.Second

// profilerProvider runs continuous CPU/memory profiling (OTLP profiles
// signal). It must be registered after the open telemetry provider: it
// resolves the shared *resource.Resource and reports its delivery metrics
// through the global meter provider.
type profilerProvider struct {
	serviceName string

	profiler *profiler.Profiler
}

var _ provider.Provider = &profilerProvider{}

func NewProfilerProvider(serviceName string) *profilerProvider {
	return &profilerProvider{serviceName: serviceName}
}

func (p *profilerProvider) Register(ctx context.Context, c provider.Container) error {
	// handlers depend on the traced profiler for trace<->profile correlation
	// even when continuous profiling is disabled, so bind it unconditionally.
	tracedProfiler := profiler.NewTracedProfiler()

	return c.Bind(func() *profiler.TracedProfiler { return tracedProfiler }, bind.Singleton())
}

func (p *profilerProvider) Boot(ctx context.Context, c provider.Container) error {
	cfg, err := profiler.ConfigFromEnv()
	if err != nil {
		return err
	}

	if !cfg.Enabled {
		return nil
	}

	var res *resource.Resource
	if err := c.Resolve(&res); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, resolve.WithParams(p.serviceName)); err != nil {
		return err
	}

	p.profiler, err = profiler.New(cfg, res, otel.GetMeterProvider(), logger)
	if err != nil {
		return err
	}

	// the profiler outlives the boot call; it is stopped on Terminate
	return p.profiler.Start(context.WithoutCancel(ctx))
}

func (p *profilerProvider) Terminate(ctx context.Context) error {
	if p.profiler == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), profilerTerminateTimeout)
	defer cancel()

	return p.profiler.Stop(ctx)
}
