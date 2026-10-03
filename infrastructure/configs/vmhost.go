package configs

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

// Where vmhost's machines' VMMs run: as host systemd units, which outlive
// vmhost's own container, or as its children, which end with it and are for
// development.
const (
	VMHostProcessModeSystemd = "systemd"
	VMHostProcessModeChild   = "child"
)

const (
	defaultVMHostListen      = "unix:///run/workload-vmhost/vmhost.sock"
	defaultVMHostSocketGroup = 10001 // the app group the orchestrators run in
	defaultVMHostDataDir     = "/var/lib/workload-vmhost"
	defaultVMHostProcessMode = VMHostProcessModeSystemd
	defaultVMHostSlice       = "workload-vm.slice"

	defaultVMHostFirecrackerBinary = "/usr/local/bin/firecracker"
	defaultVMHostKernel            = "/opt/workload-vmhost/vmlinux"
	defaultVMHostGuestBinary       = "/usr/bin/workload-guest"

	// the users machines run as: a range well clear of any a host hands out,
	// and still under the 2^31 some tools stumble at.
	defaultVMHostMachineFirstUID = 1_000_000_000
	defaultVMHostMachineUIDs     = 65536

	defaultVMHostMinMemory      = 128 << 20
	defaultVMHostMemoryOverhead = 64 << 20
	defaultVMHostMaxVMMemory    = 2 << 30
	defaultVMHostMaxVMCPU       = 2
	defaultVMHostCPUOvercommit  = 4
	defaultVMHostDiskReserve    = 20 << 30
	defaultVMHostDiskOvercommit = 3

	defaultVMHostNetworkPool = "10.250.0.0/16"
	defaultVMHostDNS         = "1.1.1.1,9.9.9.9"

	// defaultVMHostBlockedEgressPorts are the ports mining pools take
	// Stratum on, which a public VM is not let reach.
	defaultVMHostBlockedEgressPorts = "3333,4444,5555,7777,14433,14444,45560,45700"

	// defaultVMHostLogMaxSize matches the control plane's 32 MB a task.
	defaultVMHostLogMaxSize    = 32 << 20
	defaultVMHostImageCacheMax = 50 << 30
)

// WorkloadVMHost holds the configuration of the serve-workload-vmhost command:
// the privileged daemon on a KVM host that runs microVMs for the orchestrator
// on the same host. Sizes are in bytes, as they are everywhere in the workload.
type WorkloadVMHost struct {
	Listen      string `usage:"Address vmhost takes requests on: a unix socket, as unix:///path." env:"WORKLOAD_VMHOST_LISTEN" long:"listen"`
	SocketGroup int    `usage:"Group the socket is made for, mode 0660: the orchestrators' own, so nothing else on the host can ask vmhost for anything." env:"WORKLOAD_VMHOST_SOCKET_GID" long:"socket-gid"`

	DataDir string `usage:"Directory vmhost keeps its VMs, images, disks, logs and binaries in. It is the same path inside vmhost's container and on the host, since machines' VMMs run on the host, and it has to be one filesystem that allows devices." env:"WORKLOAD_VMHOST_DATA_DIR" long:"data-dir"`

	ProcessMode string `usage:"Where machines' VMMs run: systemd, as host units that outlive vmhost, or child, as vmhost's own children, which end with it and are for development." env:"WORKLOAD_VMHOST_PROCESS_MODE" long:"process-mode"`
	Slice       string `usage:"Systemd slice machines' units run in, which holds them all together." env:"WORKLOAD_VMHOST_SLICE" long:"slice"`

	NetworkNamespace string `usage:"Network namespace machines' VMMs join, as a path on the host. Empty is vmhost's own, which is the one its networks are made in." env:"WORKLOAD_VMHOST_NETNS" long:"netns"`

	FirecrackerBinary string `usage:"The firecracker binary machines run in. It is copied into the data directory, from where the host runs it." env:"WORKLOAD_VMHOST_FIRECRACKER_BINARY" long:"firecracker-binary"`
	Kernel            string `usage:"The kernel machines boot. It is copied into the data directory, from where the host runs it." env:"WORKLOAD_VMHOST_KERNEL" long:"kernel"`
	GuestBinary       string `usage:"The agent every machine boots with as its init, which an initramfs is made of." env:"WORKLOAD_VMHOST_GUEST_BINARY" long:"guest-binary"`

	MachineFirstUID int `usage:"The first of the host users machines run as. Each machine runs as one of its own, counting up from this one, with a group of the same number; nothing else may use them." env:"WORKLOAD_VMHOST_MACHINE_FIRST_UID" long:"machine-first-uid"`
	MachineUIDs     int `usage:"How many users machines may run as, which is also the most machines vmhost runs at once. Zero runs every machine as vmhost itself, which is for development and nothing else." env:"WORKLOAD_VMHOST_MACHINE_UIDS" long:"machine-uids"`

	MaxMemory      uint64  `usage:"Memory, in bytes, every VM together may be given, VMM overhead included. A VM past it is refused. It has to be set: it is the share of the host's memory VMs may have, beside everything else the host runs." env:"WORKLOAD_VMHOST_MAX_MEMORY" long:"max-memory"`
	MinMemory      uint64  `usage:"The least memory, in bytes, a VM is given, whatever its task asks for." env:"WORKLOAD_VMHOST_MIN_MEMORY" long:"min-memory"`
	MemoryOverhead uint64  `usage:"Memory, in bytes, a VM's VMM is counted as using beside its guest's." env:"WORKLOAD_VMHOST_MEMORY_OVERHEAD" long:"memory-overhead"`
	MaxVMMemory    uint64  `usage:"The most memory, in bytes, one VM may be given." env:"WORKLOAD_VMHOST_MAX_VM_MEMORY" long:"max-vm-memory"`
	MaxVMCPU       float64 `usage:"The most CPUs one VM may be given." env:"WORKLOAD_VMHOST_MAX_VM_CPU" long:"max-vm-cpu"`
	CPUOvercommit  float64 `usage:"How many of the VMs' CPUs there may be for each of the host's." env:"WORKLOAD_VMHOST_CPU_OVERCOMMIT" long:"cpu-overcommit"`
	DiskReserve    uint64  `usage:"Disk, in bytes, kept free on the data directory's filesystem whatever VMs ask for." env:"WORKLOAD_VMHOST_DISK_RESERVE" long:"disk-reserve"`
	DiskOvercommit float64 `usage:"How many times the free disk the VMs' scratch disks may add up to. They are sparse, so they rarely fill." env:"WORKLOAD_VMHOST_DISK_OVERCOMMIT" long:"disk-overcommit"`

	NetworkPool        string `usage:"Addresses VMs' networks are carved out of, a /24 each. They must be vmhost's alone." env:"WORKLOAD_VMHOST_NETWORK_POOL" long:"network-pool"`
	DNS                string `usage:"Nameservers a VM that reaches the internet is given, separated by commas. One that does not is given none." env:"WORKLOAD_VMHOST_DNS" long:"dns"`
	BlockedEgressPorts string `usage:"Ports no VM may reach on the internet, separated by commas." env:"WORKLOAD_VMHOST_BLOCKED_EGRESS_PORTS" long:"blocked-egress-ports"`

	LogMaxSize        uint64 `usage:"How much of its output, in bytes, vmhost keeps for one VM." env:"WORKLOAD_VMHOST_LOG_MAX_SIZE" long:"log-max-size"`
	AllowedRegistries string `usage:"Registries VM images may come from, separated by commas. Empty allows any." env:"WORKLOAD_VMHOST_ALLOWED_REGISTRIES" long:"allowed-registries"`
	ImageCacheMax     uint64 `usage:"How much disk, in bytes, the images VMs boot may take, past which the least recently used that nothing boots are let go." env:"WORKLOAD_VMHOST_IMAGE_CACHE_MAX" long:"image-cache-max"`
}

// NewWorkloadVMHost returns the configuration of the serve-workload-vmhost
// command, holding the defaults it runs with until the console overrides them.
func NewWorkloadVMHost() *WorkloadVMHost {
	return &WorkloadVMHost{
		Listen:             defaultVMHostListen,
		SocketGroup:        defaultVMHostSocketGroup,
		DataDir:            defaultVMHostDataDir,
		ProcessMode:        defaultVMHostProcessMode,
		Slice:              defaultVMHostSlice,
		FirecrackerBinary:  defaultVMHostFirecrackerBinary,
		Kernel:             defaultVMHostKernel,
		GuestBinary:        defaultVMHostGuestBinary,
		MachineFirstUID:    defaultVMHostMachineFirstUID,
		MachineUIDs:        defaultVMHostMachineUIDs,
		MinMemory:          defaultVMHostMinMemory,
		MemoryOverhead:     defaultVMHostMemoryOverhead,
		MaxVMMemory:        defaultVMHostMaxVMMemory,
		MaxVMCPU:           defaultVMHostMaxVMCPU,
		CPUOvercommit:      defaultVMHostCPUOvercommit,
		DiskReserve:        defaultVMHostDiskReserve,
		DiskOvercommit:     defaultVMHostDiskOvercommit,
		NetworkPool:        defaultVMHostNetworkPool,
		DNS:                defaultVMHostDNS,
		BlockedEgressPorts: defaultVMHostBlockedEgressPorts,
		LogMaxSize:         defaultVMHostLogMaxSize,
		ImageCacheMax:      defaultVMHostImageCacheMax,
	}
}

// SocketPath is the unix socket vmhost takes requests on.
func (c *WorkloadVMHost) SocketPath() (string, error) {
	parsed, err := url.Parse(c.Listen)
	if err != nil || parsed.Scheme != "unix" || len(parsed.Path) == 0 {
		return "", fmt.Errorf("%q is not a unix socket vmhost can listen on: it is written unix:///path", c.Listen)
	}

	return parsed.Path, nil
}

// Pool is the addresses VMs' networks are carved out of.
func (c *WorkloadVMHost) Pool() (*net.IPNet, error) {
	_, pool, err := net.ParseCIDR(c.NetworkPool)
	if err != nil {
		return nil, fmt.Errorf("%q is not a network pool: %w", c.NetworkPool, err)
	}

	return pool, nil
}

// Nameservers are the nameservers a VM that reaches the internet is given.
func (c *WorkloadVMHost) Nameservers() []string {
	return commaSeparated(c.DNS)
}

// BlockedPorts are the ports no VM may reach on the internet.
func (c *WorkloadVMHost) BlockedPorts() ([]uint16, error) {
	items := commaSeparated(c.BlockedEgressPorts)

	ports := make([]uint16, 0, len(items))
	for _, item := range items {
		port, err := strconv.ParseUint(item, 10, 16)
		if err != nil || port == 0 {
			return nil, fmt.Errorf("%q is not a port", item)
		}

		ports = append(ports, uint16(port))
	}

	return ports, nil
}

// Registries are the registries VM images may come from; none is any.
func (c *WorkloadVMHost) Registries() []string {
	return commaSeparated(c.AllowedRegistries)
}
