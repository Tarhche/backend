package workload

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/danceable/provider"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/hostports"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/journal"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/state"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	microsandboxAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/microsandbox"
)

const (
	// MicrosandboxHealth is the handler of the service's plain healthcheck,
	// which the serve command serves on loopback, apart from the API.
	MicrosandboxHealth = "workload:microsandbox:health"

	microsandboxLoggerName = "workload-microsandbox"

	// cgroupRoot is where the container's cgroup is mounted.
	cgroupRoot = "/sys/fs/cgroup"

	// unlimited is where a cgroup v1 memory limit stops being one: the
	// kernel reports "no limit" as the largest page-aligned int64.
	unlimited = 1 << 62
)

// microsandboxProvider builds the workload-microsandbox service: the stores of
// its runs, the run supervisor over the microsandbox the SDK's provider bound
// as runs.Sandboxes, the API's handler, and the healthcheck's.
//
// The SDK's provider is the only part of the service built with the
// microsandbox tag. This one is static, so everything it wires is built and
// tested by every build, against a fake microsandbox in the tests.
type microsandboxProvider struct {
	// cgroupRoot is where the container's memory limit is read from.
	cgroupRoot string

	journal *journal.Journal
}

var _ provider.Provider = &microsandboxProvider{}

func NewMicrosandboxProvider() *microsandboxProvider {
	return &microsandboxProvider{cgroupRoot: cgroupRoot}
}

func (p *microsandboxProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *microsandboxProvider) Boot(ctx context.Context, c provider.Container) error {
	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams(microsandboxLoggerName)); err != nil {
		return err
	}

	var microsandboxConfigs *configs.WorkloadMicrosandbox
	if err := c.Resolve(&microsandboxConfigs); err != nil {
		return err
	}

	// microsandbox itself, as the SDK's provider bound it.
	var sandboxes runs.Sandboxes
	if err := c.Resolve(&sandboxes); err != nil {
		return err
	}

	var tracedProfiler *profiler.TracedProfiler
	if err := c.Resolve(&tracedProfiler); err != nil {
		return err
	}

	limit, err := containerMemoryLimit(p.cgroupRoot)
	if err != nil {
		return err
	}

	budget, err := microsandboxConfigs.Budget(limit)
	if err != nil {
		return err
	}

	first, last, err := microsandboxConfigs.HostPorts()
	if err != nil {
		return err
	}

	allocator, err := hostports.New(first, last, microsandboxConfigs.PortBindAddress)
	if err != nil {
		return err
	}

	records, err := state.NewStore(filepath.Join(microsandboxConfigs.StateDir, "runs"), logger)
	if err != nil {
		return err
	}

	journals, err := journal.New(filepath.Join(microsandboxConfigs.StateDir, "logs"), journal.DefaultCapacity)
	if err != nil {
		return err
	}

	p.journal = journals

	config := runs.DefaultConfig()
	config.Budget = budget
	config.MemoryFloor = microsandboxConfigs.MinMemory
	config.BindAddress = microsandboxConfigs.PortBindAddress
	config.Nameservers = microsandboxConfigs.NameserverList()
	config.ServiceVersion = serviceVersion()

	supervisor := runs.New(sandboxes, records, journals, allocator, config, logger)

	logger.Info("the memory budget for VMs", "bytes", budget, "container_limit", limit)

	if err := c.Bind(func() *runs.Supervisor { return supervisor }, provider.Singleton()); err != nil {
		return err
	}

	// the healthcheck asks whether the service is ready, which is whether it
	// can run anything at all: a service that answers but cannot is not
	// healthy, and a deploy waiting for it fails rather than carries on.
	health := http.NewServeMux()
	health.Handle("GET /health", healthAPI.NewHealthHandler(checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "microsandbox", Pinger: supervisor},
	)))

	if err := c.Bind(func() http.Handler { return health }, provider.Singleton(), provider.WithName(MicrosandboxHealth)); err != nil {
		return err
	}

	handler := microsandboxHandler(supervisor, tracedProfiler, logger)

	return c.Bind(func() http.Handler { return handler }, provider.Singleton())
}

func (p *microsandboxProvider) Terminate(ctx context.Context) error {
	if p.journal == nil {
		return nil
	}

	return p.journal.Close()
}

// microsandboxHandler is the API, inside the middleware every service has.
//
// Requests are logged at debug level unless they fail: the orchestrators ask
// for their runs and their stats several times a second, which would bury
// everything else.
func microsandboxHandler(supervisor *runs.Supervisor, tracedProfiler *profiler.TracedProfiler, logger *slog.Logger) http.Handler {
	logConfig := middleware.DefaultLogConfig()
	logConfig.DefaultLevel = slog.LevelDebug

	return middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/workload/microsandbox",
				// inside Telemetry so profile samples link to the request span
				middleware.NewProfilingMiddleware(
					middleware.NewLogMiddlewareWithConfig(
						microsandboxAPI.NewHandler(supervisor, logger),
						logger,
						logConfig,
					),
					tracedProfiler,
				),
			),
		),
		logger,
	)
}

// containerMemoryLimit is the memory limit of the container the service runs
// in, in bytes, and zero when it has none. It reads cgroup v2's memory.max,
// and cgroup v1's memory.limit_in_bytes where there is no v2.
func containerMemoryLimit(root string) (uint64, error) {
	for _, file := range []string{"memory.max", filepath.Join("memory", "memory.limit_in_bytes")} {
		data, err := os.ReadFile(filepath.Join(root, file))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return 0, fmt.Errorf("the container's memory limit could not be read: %w", err)
		}

		value := strings.TrimSpace(string(data))
		if value == "max" {
			return 0, nil
		}

		limit, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("the container's memory limit %q is not a number", value)
		}

		if limit >= unlimited {
			return 0, nil
		}

		return limit, nil
	}

	return 0, nil
}

// serviceVersion is the build of the service, as the module or its commit
// says.
func serviceVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	var revision, modified string

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}

	if len(revision) == 0 {
		return info.Main.Version
	}

	if len(revision) > 12 {
		revision = revision[:12]
	}

	if modified == "true" {
		revision += "-dirty"
	}

	return revision
}
