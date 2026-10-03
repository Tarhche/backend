package workload

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/danceable/provider"
	"go.opentelemetry.io/otel"

	vmhostEngine "github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/createVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/deleteImage"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/deleteVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/dialVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/endExec"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/ensureNetwork"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/execVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getImages"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getInfo"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMLogs"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMStats"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMs"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/killVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/prepareImage"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/reconcile"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/removeNetwork"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/restartVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/startVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/stopVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/vmstate"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/fabric"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/firecracker"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/guest"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/image"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/initrd"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/output"
	vmhostAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/vmhost/api"
)

const (
	// VMHostHandler is vmhost's API, served on its socket.
	VMHostHandler = "workload:vmhost:handler"

	// VMHostLogger is the name vmhost's logger is resolved with.
	VMHostLogger = "workload-vmhost"

	// vmhostMeter is the instrumentation scope vmhost measures under.
	vmhostMeter = "github.com/khanzadimahdi/testproject/application/workload/vmhost"

	// vmhostCloseTimeout bounds letting go of the VMs when vmhost stops: they
	// keep running, and only the keepers have to let go.
	vmhostCloseTimeout = 10 * time.Second
)

// vmhostProvider builds vmhost: the data directory laid out, what machines
// boot put in it, the parts VMs are made with, the engine that drives them,
// and the API that takes requests for them.
type vmhostProvider struct {
	engine *vmhostEngine.Engine
}

var _ provider.Provider = &vmhostProvider{}

func NewVMHostProvider() *vmhostProvider {
	return &vmhostProvider{}
}

func (p *vmhostProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *vmhostProvider) Boot(ctx context.Context, c provider.Container) error {
	var vmhostConfigs *configs.WorkloadVMHost
	if err := c.Resolve(&vmhostConfigs); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams(VMHostLogger)); err != nil {
		return err
	}

	var validator domain.Validator
	if err := c.Resolve(&validator); err != nil {
		return err
	}

	engine, err := p.buildEngine(vmhostConfigs, logger)
	if err != nil {
		return err
	}

	p.engine = engine

	handler := vmhostAPI.NewHandler(vmhostAPI.UseCases{
		GetInfo:       getInfo.NewUseCase(engine),
		PrepareImage:  prepareImage.NewUseCase(engine, validator),
		GetImages:     getImages.NewUseCase(engine),
		DeleteImage:   deleteImage.NewUseCase(engine, validator),
		EnsureNetwork: ensureNetwork.NewUseCase(engine, validator),
		RemoveNetwork: removeNetwork.NewUseCase(engine, validator),
		CreateVM: createVM.NewUseCase(engine, validator, createVM.Limits{
			MaxMemory: vmhostConfigs.MaxVMMemory,
			MaxCPU:    vmhostConfigs.MaxVMCPU,
		}),
		GetVMs:     getVMs.NewUseCase(engine, validator),
		GetVM:      getVM.NewUseCase(engine, validator),
		StartVM:    startVM.NewUseCase(engine, validator),
		StopVM:     stopVM.NewUseCase(engine, validator),
		RestartVM:  restartVM.NewUseCase(engine, validator),
		KillVM:     killVM.NewUseCase(engine, validator),
		DeleteVM:   deleteVM.NewUseCase(engine, validator),
		GetVMLogs:  getVMLogs.NewUseCase(engine, validator),
		GetVMStats: getVMStats.NewUseCase(engine, validator),
		ExecVM:     execVM.NewUseCase(engine, validator),
		EndExec:    endExec.NewUseCase(engine, validator),
		DialVM:     dialVM.NewUseCase(engine, validator),
	}, logger)

	if err := c.Bind(func() http.Handler { return handler }, provider.Singleton(), provider.WithName(VMHostHandler)); err != nil {
		return err
	}

	reconciling := reconcile.NewUseCase(engine)

	return c.Bind(func() *reconcile.UseCase { return reconciling }, provider.Singleton())
}

// buildEngine lays out the data directory, puts in it what machines boot, and
// makes the engine of vmhost's parts.
func (p *vmhostProvider) buildEngine(vmhostConfigs *configs.WorkloadVMHost, logger *slog.Logger) (*vmhostEngine.Engine, error) {
	// the budget is the share of the host's memory VMs may have, beside
	// everything else the host runs: there is no default that fits every
	// host, and none at all is no VM at all.
	if vmhostConfigs.MaxMemory == 0 {
		return nil, errors.New("WORKLOAD_VMHOST_MAX_MEMORY has to be set: it is the memory, in bytes, every VM together may be given")
	}

	mode := firecracker.Mode(vmhostConfigs.ProcessMode)
	if mode != firecracker.ModeSystemd && mode != firecracker.ModeChild {
		return nil, fmt.Errorf("%q is not where machines run: it is %s or %s", vmhostConfigs.ProcessMode, firecracker.ModeSystemd, firecracker.ModeChild)
	}

	dataDir := vmhostConfigs.DataDir

	if err := layout.Prepare(dataDir); err != nil {
		return nil, err
	}

	// a machine's unit is the host's, and runs what is in the data directory
	// rather than what is in vmhost's image: put in by what it holds, an
	// upgrade never replaces one a running machine was started from.
	binary, err := layout.Install(layout.Bin(dataDir), vmhostConfigs.FirecrackerBinary, "firecracker", 0o755)
	if err != nil {
		return nil, err
	}

	kernel, err := layout.Install(layout.Boot(dataDir), vmhostConfigs.Kernel, "vmlinux", 0o644)
	if err != nil {
		return nil, err
	}

	initramfs, err := initrd.Ensure(layout.Boot(dataDir), vmhostConfigs.GuestBinary)
	if err != nil {
		return nil, err
	}

	pool, err := vmhostConfigs.Pool()
	if err != nil {
		return nil, err
	}

	blockedPorts, err := vmhostConfigs.BlockedPorts()
	if err != nil {
		return nil, err
	}

	// machines join the namespace their taps are in: vmhost's own, which is
	// the holder container's it shares. vmhost runs with the host's pids, so
	// its own pid names that namespace on the host too, where a machine's
	// unit opens it.
	networkNamespace := vmhostConfigs.NetworkNamespace
	if len(networkNamespace) == 0 {
		networkNamespace = fmt.Sprintf("/proc/%d/ns/net", os.Getpid())
	}

	hypervisor, err := firecracker.New(firecracker.Config{
		DataDir:          dataDir,
		Binary:           binary,
		Mode:             mode,
		Slice:            vmhostConfigs.Slice,
		NetworkNamespace: networkNamespace,
		FirstUID:         vmhostConfigs.MachineFirstUID,
		UIDs:             vmhostConfigs.MachineUIDs,
		MemoryOverhead:   vmhostConfigs.MemoryOverhead,
	}, logger)
	if err != nil {
		return nil, err
	}

	images, err := image.NewStore(image.Config{
		Dir:        layout.Images(dataDir),
		Registries: vmhostConfigs.Registries(),
		CacheMax:   vmhostConfigs.ImageCacheMax,
	}, logger)
	if err != nil {
		return nil, err
	}

	networks, err := fabric.New(fabric.Config{
		Dir:          layout.Fabric(dataDir),
		Pool:         pool,
		BlockedPorts: blockedPorts,
		FirstUID:     vmhostConfigs.MachineFirstUID,
		UIDs:         vmhostConfigs.MachineUIDs,
	}, logger)
	if err != nil {
		return nil, err
	}

	states, err := vmstate.Open(dataDir)
	if err != nil {
		return nil, err
	}

	return vmhostEngine.New(vmhostEngine.Config{
		Version:        vmhostVersion(),
		ProcessMode:    vmhostConfigs.ProcessMode,
		Architecture:   runtime.GOARCH,
		Kernel:         kernel,
		Initrd:         initramfs,
		ScratchPath:    func(id string) string { return layout.Scratch(dataDir, id) },
		MaxMemory:      vmhostConfigs.MaxMemory,
		MinMemory:      vmhostConfigs.MinMemory,
		MemoryOverhead: vmhostConfigs.MemoryOverhead,
		MaxVMMemory:    vmhostConfigs.MaxVMMemory,
		MaxVMCPU:       vmhostConfigs.MaxVMCPU,
		HostCPUs:       runtime.NumCPU(),
		CPUOvercommit:  vmhostConfigs.CPUOvercommit,
		DiskReserve:    vmhostConfigs.DiskReserve,
		DiskOvercommit: vmhostConfigs.DiskOvercommit,
		FirstUID:       vmhostConfigs.MachineFirstUID,
		UIDs:           vmhostConfigs.MachineUIDs,
		Nameservers:    vmhostConfigs.Nameservers(),
		ImageCacheMax:  vmhostConfigs.ImageCacheMax,
	}, vmhostEngine.Parts{
		Hypervisor: hypervisor,
		Images:     images,
		Fabric:     networks,
		Guests:     guest.NewConnector(),
		States:     states,
		Logs:       output.NewStore(dataDir, vmhostConfigs.LogMaxSize),
		FreeSpace:  func() (uint64, error) { return layout.FreeSpace(dataDir) },
		Metrics:    otel.Meter(vmhostMeter),
	}, logger)
}

// Terminate lets go of the VMs, leaving every one of them as it is: they are
// the host's, and the next vmhost takes them back.
func (p *vmhostProvider) Terminate(ctx context.Context) error {
	if p.engine == nil {
		return nil
	}

	closing, cancel := context.WithTimeout(context.WithoutCancel(ctx), vmhostCloseTimeout)
	defer cancel()

	return p.engine.Close(closing)
}

// vmhostVersion is the application's version, as the build says it: the
// commit it was built from, or the module's version, or that it is a
// development build.
func vmhostVersion() string {
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

	if len(revision) > 0 {
		revision = revision[:min(12, len(revision))]
		if modified == "true" {
			revision += "-dirty"
		}

		return revision
	}

	if len(info.Main.Version) > 0 && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	return "devel"
}
