// Package vmhost wires a vmhost: the node's engine, made from the vmhost's
// settings, and the API that serves it to the orchestrator beside it.
//
// It imports nothing that wires another service.
package vmhost

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/microsandbox"
	vmhostAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/vmhost"
)

const (
	// Logger names the vmhost's logger, which the serve command resolves.
	Logger = "workload:vmhost:logger"

	// loggerName is what the vmhost's telemetry calls its logger.
	loggerName = "workload-vmhost"

	// ShutdownTimeout is how long stopping every VM is given. It is within
	// the container's stop_grace_period of 120 s, which leaves the rest of it
	// to everything else that is let go of on the way out.
	ShutdownTimeout = 110 * time.Second

	mebibyte = 1 << 20

	// reserved is the memory kept back from VMs when the budget is left to
	// the vmhost: what the vmhost and the engine need for themselves.
	reserved = 512 * mebibyte
)

// vmhostProvider makes the node's engine, and the API that serves it.
type vmhostProvider struct {
	engine *engine
}

var _ provider.Provider = &vmhostProvider{}

func NewProvider() *vmhostProvider {
	return &vmhostProvider{}
}

func (p *vmhostProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

// Boot makes the engine from the vmhost's settings and binds it, the API that
// serves it, and the vmhost's logger.
func (p *vmhostProvider) Boot(ctx context.Context, c provider.Container) error {
	var settings *configs.WorkloadVMHost
	if err := c.Resolve(&settings); err != nil {
		return err
	}

	var telemetry *slog.Logger
	if err := c.Resolve(&telemetry, provider.WithParams(loggerName)); err != nil {
		return err
	}

	// what the vmhost says goes to its container's own log as well as to
	// the telemetry: it is the one service whose restarts stop VMs, and what
	// it said on the way down is what says why, whether or not the telemetry
	// was there to hear it.
	logger := slog.New(slog.NewMultiHandler(
		telemetry.Handler(),
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}),
	))

	options, err := Options(settings, NewMachine("/"), logger)
	if err != nil {
		return err
	}

	started, err := microsandbox.New(ctx, options)
	if err != nil {
		return fmt.Errorf("the engine cannot be started: %w", err)
	}

	p.engine = &engine{Engine: started}

	server := vmhostAPI.NewServer(p.engine, logger)

	if err := c.Bind(func() microsandbox.Engine { return p.engine }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() *vmhostAPI.Server { return server }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() *slog.Logger { return logger }, provider.Singleton(), provider.WithName(Logger))
}

// Terminate stops every VM the engine holds, when the serve command did not
// get to: one that ran has stopped them already. VMs left running would be
// killed with the container, losing what their guests had not yet written.
func (p *vmhostProvider) Terminate(ctx context.Context) error {
	if p.engine == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ShutdownTimeout)
	defer cancel()

	return p.engine.Shutdown(ctx)
}

// engine is an engine that is shut down once, by whichever of the serve
// command and the provider gets there first.
type engine struct {
	microsandbox.Engine

	once sync.Once
	err  error
}

func (e *engine) Shutdown(ctx context.Context) error {
	e.once.Do(func() {
		e.err = e.Engine.Shutdown(ctx)
	})

	return e.err
}

// Host is what a vmhost reads off where it runs when its settings leave the
// budget to it.
type Host interface {
	// CPUs are the CPUs this container may use.
	CPUs() uint

	// Memory is the memory this container may use, in bytes.
	Memory() (uint64, error)
}

// Options are what the engine is given, from the vmhost's settings.
//
// The addresses are IPs rather than names: published ports are bound to the
// vmhost's own, and a VM takes connections from the orchestrator's alone, both
// of which an engine holds as addresses. A budget the settings leave at zero
// is what this container may use: its CPUs, and 80% of its memory less what
// the vmhost keeps for itself.
func Options(settings *configs.WorkloadVMHost, host Host, logger *slog.Logger) (microsandbox.Options, error) {
	first, last, err := portRange(settings.PortRange)
	if err != nil {
		return microsandbox.Options{}, err
	}

	if net.ParseIP(settings.AdvertiseHost) == nil {
		return microsandbox.Options{}, fmt.Errorf("WORKLOAD_VMHOST_ADVERTISE_HOST is this vmhost's own IP on its pair network, which VMs' ports are published on; got %q", settings.AdvertiseHost)
	}

	if net.ParseIP(settings.OrchestratorAddress) == nil {
		return microsandbox.Options{}, fmt.Errorf("WORKLOAD_VMHOST_ORCHESTRATOR_IP is the IP of this vmhost's orchestrator on their pair network, the only address VMs take connections from; got %q", settings.OrchestratorAddress)
	}

	if len(settings.Home) == 0 {
		return microsandbox.Options{}, fmt.Errorf("MSB_HOME is where the engine keeps its VMs, and has to be said")
	}

	if len(settings.DockerImage) == 0 {
		return microsandbox.Options{}, fmt.Errorf("WORKLOAD_VMHOST_DOCKER_IMAGE is what a Docker VM boots from, and has to be said")
	}

	if settings.Disk == 0 {
		return microsandbox.Options{}, fmt.Errorf("WORKLOAD_VMHOST_DISK is the disk this node offers to VMs, which cannot be none")
	}

	if settings.MaxConcurrentBoots == 0 {
		return microsandbox.Options{}, fmt.Errorf("WORKLOAD_VMHOST_MAX_CONCURRENT_BOOTS is how many VMs may boot at once, which cannot be none")
	}

	capacity := vm.Resources{CPUs: settings.CPUs, Memory: settings.Memory, Disk: settings.Disk}

	if capacity.CPUs == 0 {
		capacity.CPUs = host.CPUs()
	}

	if capacity.Memory == 0 {
		capacity.Memory, err = memoryBudget(host)
		if err != nil {
			return microsandbox.Options{}, err
		}
	}

	return microsandbox.Options{
		Home:                settings.Home,
		BindAddress:         settings.AdvertiseHost,
		OrchestratorAddress: settings.OrchestratorAddress,
		FirstPort:           first,
		LastPort:            last,
		DockerImage:         settings.DockerImage,
		Capacity:            capacity,
		MaxConcurrentBoots:  settings.MaxConcurrentBoots,
		Logger:              logger,
	}, nil
}

// memoryBudget is the memory offered to VMs when the settings leave it to the
// vmhost: 80% of what this container may use, less what the vmhost keeps for
// itself.
func memoryBudget(host Host) (uint64, error) {
	limit, err := host.Memory()
	if err != nil {
		return 0, fmt.Errorf("how much memory this container may use cannot be read, so WORKLOAD_VMHOST_MEMORY has to be said: %w", err)
	}

	budget := limit / 10 * 8
	if budget <= reserved {
		return 0, fmt.Errorf("this container may use %d MiB of memory, which leaves VMs nothing once the vmhost has its %d MiB: raise its limit or say WORKLOAD_VMHOST_MEMORY", limit/mebibyte, reserved/mebibyte)
	}

	return budget - reserved, nil
}

// portRange reads first-last.
func portRange(value string) (port.Port, port.Port, error) {
	from, to, found := strings.Cut(value, "-")
	if !found {
		return 0, 0, fmt.Errorf("WORKLOAD_VMHOST_PORT_RANGE is the host ports VMs' ports are published on, as first-last; got %q", value)
	}

	first, err := portNumber(from)
	if err != nil {
		return 0, 0, fmt.Errorf("WORKLOAD_VMHOST_PORT_RANGE starts at %q: %w", from, err)
	}

	last, err := portNumber(to)
	if err != nil {
		return 0, 0, fmt.Errorf("WORKLOAD_VMHOST_PORT_RANGE ends at %q: %w", to, err)
	}

	if first > last {
		return 0, 0, fmt.Errorf("WORKLOAD_VMHOST_PORT_RANGE starts at %d, after it ends at %d", first, last)
	}

	return first, last, nil
}

func portNumber(value string) (port.Port, error) {
	number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 16)
	if err != nil || number == 0 {
		return 0, fmt.Errorf("a port is a number from 1 to 65535")
	}

	return port.Port(number), nil
}
