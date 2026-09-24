package configs

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
)

const (
	defaultRunnerControlPlanePort   = 80
	defaultRunnerOrchestratorPort   = 80
	defaultRunnerIngressPort        = 80
	defaultRunnerIngressDomain      = "runner.localhost"
	defaultRunnerMaxLogBytes        = 32 << 20 // 32 MB per task
	defaultRunnerOrchestratorCpu    = 0.5
	defaultRunnerOrchestratorMemory = 256 << 20 // 256 MB
	defaultRunnerOrchestratorDisk   = 256 << 20 // 256 MB

	defaultRunnerTunnelPort = 81

	defaultRunnerTunnelMinConnections = 2
	defaultRunnerTunnelMaxConnections = 4
	defaultRunnerTunnelMaxIdleTime    = 2 * time.Minute

	defaultTunnelMaxStreamsPerSession       = 256
	defaultTunnelMaxSessionsPerOrchestrator = 8
)

// The runtimes an orchestrator can run its tasks on.
const (
	RuntimeFirecracker = "firecracker"
	RuntimeDocker      = "docker"

	defaultRunnerRuntime = RuntimeFirecracker
)

// Where firecracker's machines are made, and what they are made with.
const (
	defaultRunnerStateDir       = "/var/lib/runner"
	defaultRunnerLauncherSocket = defaultRunnerStateDir + "/launcher.sock"
	defaultRunnerGuestBinary    = "/usr/bin/runner-guest"
	defaultRunnerKernel         = "/opt/runner/vmlinux"
	defaultRunnerNameservers    = "1.1.1.1,8.8.8.8"
	defaultRunnerNetworkPool    = "10.200.0.0/16"

	// the uid and gid the app runs as in its image, which is who the
	// orchestrators are.
	defaultRunnerOrchestratorUID = 10000
	defaultRunnerOrchestratorGID = 10001

	// the users machines run as: a range well clear of any a host hands out,
	// and still under the 2^31 some tools stumble at.
	defaultRunnerMachineFirstUID = 1_000_000_000
	defaultRunnerMachineUIDs     = 65536

	defaultRunnerLauncherHealthPort   = 8050
	defaultRunnerFirecrackerBinary    = "/usr/local/bin/firecracker"
	defaultRunnerLauncherMaxVCPUs     = 2
	defaultRunnerLauncherMaxMemoryMiB = 2048
)

// RunnerControlPlane holds the configuration of the serve-runner-controlplane command.
type RunnerControlPlane struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	MaxLogBytes int64 `usage:"How much log one task may keep. Past it, further lines are dropped rather than stored." env:"RUNNER_MAX_LOG_BYTES" long:"max-log-bytes"`

	DefaultCpu    float64 `usage:"CPUs a task is limited to when its specification names no limit." env:"RUNNER_DEFAULT_CPU" long:"default-cpu"`
	DefaultMemory uint64  `usage:"Memory, in bytes, a task is limited to when its specification names no limit." env:"RUNNER_DEFAULT_MEMORY" long:"default-memory"`
	DefaultDisk   uint64  `usage:"Disk, in bytes, a task is limited to when its specification names no limit." env:"RUNNER_DEFAULT_DISK" long:"default-disk"`
}

// NewRunnerControlPlane returns the configuration of the serve-runner-controlplane
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerControlPlane() *RunnerControlPlane {
	return &RunnerControlPlane{
		Port:          defaultRunnerControlPlanePort,
		MaxLogBytes:   defaultRunnerMaxLogBytes,
		DefaultCpu:    defaultRunnerOrchestratorCpu,
		DefaultMemory: defaultRunnerOrchestratorMemory,
		DefaultDisk:   defaultRunnerOrchestratorDisk,
	}
}

// RunnerIngress holds the configuration of the serve-runner-ingress command.
type RunnerIngress struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	Domain string `usage:"Domain a task's exposed ports are served on, without a leading dot. A request to a hostname under it is routed to the task the hostname names." env:"RUNNER_INGRESS_DOMAIN" long:"domain"`

	TunnelPort int `usage:"Port the orchestrators open their connections to. It carries nothing but them, so it is not the port requests arrive on." env:"RUNNER_TUNNEL_PORT" long:"tunnel-port"`

	TunnelAuthority   string `usage:"The certificate authority, in PEM form, an orchestrator's own certificate has to be signed by. The authority's private key is never needed here." env:"RUNNER_TUNNEL_CA_CERT" long:"tunnel-ca-cert"`
	TunnelCertificate string `usage:"The certificate, in PEM form, the ingress answers with." env:"RUNNER_TUNNEL_CERT" long:"tunnel-cert"`
	TunnelKey         string `usage:"The private key, in PEM form, for that certificate." env:"RUNNER_TUNNEL_KEY" long:"tunnel-key"`

	TunnelIdentitySuffix       string `usage:"Domain an orchestrator's certificate carries its name under, dropped to leave the name. Empty takes the first subject alternative name whole." env:"RUNNER_TUNNEL_IDENTITY_SUFFIX" long:"tunnel-identity-suffix"`
	TunnelAllowedOrchestrators string `usage:"Orchestrators allowed to connect, separated by commas. Empty allows every orchestrator the authority signed for." env:"RUNNER_TUNNEL_ALLOWED_ORCHESTRATORS" long:"tunnel-allowed-orchestrators"`

	TunnelMaxStreamsPerSession       int `usage:"How many client connections one of an orchestrator's connections will carry before the next is used. It is a blast radius before it is a capacity: they all end when it does." env:"RUNNER_TUNNEL_MAX_STREAMS_PER_SESSION" long:"tunnel-max-streams-per-session"`
	TunnelMaxSessionsPerOrchestrator int `usage:"How many connections one orchestrator may hold here." env:"RUNNER_TUNNEL_MAX_SESSIONS_PER_ORCHESTRATOR" long:"tunnel-max-sessions-per-orchestrator"`

	ForwardedPorts string `usage:"Ports to carry arbitrary TCP into the tunnel on, as listen=orchestrator:target, separated by commas — 8022=orchestrator-a:22 reaches port 22 on that orchestrator, 9000=:api reaches the api service on whichever orchestrator the router picks. Nothing is forwarded by default." env:"RUNNER_INGRESS_FORWARDS" long:"forward"`
}

// NewRunnerIngress returns the configuration of the serve-runner-ingress
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerIngress() *RunnerIngress {
	return &RunnerIngress{
		Port:                             defaultRunnerIngressPort,
		Domain:                           defaultRunnerIngressDomain,
		TunnelPort:                       defaultRunnerTunnelPort,
		TunnelMaxStreamsPerSession:       defaultTunnelMaxStreamsPerSession,
		TunnelMaxSessionsPerOrchestrator: defaultTunnelMaxSessionsPerOrchestrator,
	}
}

// AllowedOrchestrators is the orchestrators this ingress will take, or none named at all,
// which allows every orchestrator the authority signed for.
func (c *RunnerIngress) AllowedOrchestrators() []string {
	return commaSeparated(c.TunnelAllowedOrchestrators)
}

// RunnerOrchestrator holds the configuration of the serve-runner-orchestrator command.
type RunnerOrchestrator struct {
	Port int    `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`
	Name string `usage:"specifies the unique name of the orchestrator." env:"RUNNER_ORCHESTRATOR_NAME" long:"name" short:"n"`

	// Runtime is what runs this orchestrator's tasks. Everything above it asks
	// the same things of either; only what each needs to be told differs.
	Runtime string `usage:"What runs the tasks: firecracker, for a microVM each, or docker, for a container each." env:"RUNNER_RUNTIME" long:"runtime"`

	DockerHost string `usage:"Docker daemon the tasks are run on, when docker runs them. Empty uses the Docker client's own default." env:"DOCKER_HOST" long:"docker-host"`

	// DockerAdvertiseHost is where this orchestrator reaches the ports docker
	// publishes its tasks on. It is the docker daemon's host rather than this
	// service's, which are not the same machine when the daemon is a service of
	// its own.
	DockerAdvertiseHost string `usage:"Host this orchestrator reaches its tasks' published ports at when docker runs them, which is the docker daemon's own rather than this one." env:"RUNNER_DOCKER_ADVERTISE_HOST" long:"docker-advertise-host"`

	// StateDir is where firecracker's machines are made, shared with the
	// launcher at the same path on both sides: what the orchestrator makes
	// there, the launcher links into a machine by the same name.
	StateDir string `usage:"Directory machines are made in when firecracker runs them, shared with the launcher at the same path on both sides." env:"RUNNER_STATE_DIR" long:"state-dir"`

	LauncherSocket string `usage:"Unix socket the launcher takes orders on, when firecracker runs the tasks." env:"RUNNER_LAUNCHER_SOCKET" long:"launcher-socket"`
	Kernel         string `usage:"Kernel machines boot. Empty is the one the launcher installs in the state directory." env:"RUNNER_KERNEL" long:"kernel"`
	GuestBinary    string `usage:"The agent every machine boots with as its init." env:"RUNNER_GUEST_BINARY" long:"guest-binary"`
	Nameservers    string `usage:"Nameservers a machine that routes out is given, separated by commas." env:"RUNNER_NAMESERVERS" long:"nameservers"`

	// OrchestratorUID and OrchestratorGID are who the orchestrators run as.
	// What an orchestrator makes while it runs as root is made theirs, as the
	// launcher expects it to be.
	OrchestratorUID int `usage:"Who the orchestrators run as. What an orchestrator makes while it runs as root is made theirs." env:"RUNNER_ORCHESTRATOR_UID" long:"orchestrator-uid"`
	OrchestratorGID int `usage:"The orchestrators' group." env:"RUNNER_ORCHESTRATOR_GID" long:"orchestrator-gid"`

	// PublicKey verifies the tokens the blog signs. An orchestrator never mints one,
	// so it is given the public half and nothing else.
	PublicKey string `usage:"ECDSA public key, in PEM form, the access tokens are verified against. It is the public half of the key the blog signs them with." env:"PUBLIC_KEY" long:"public-key"`

	TunnelAddresses string `usage:"host:port of every ingress this orchestrator opens connections to, separated by commas. It keeps a pool at each, so it is reachable through all of them." env:"RUNNER_TUNNEL_ADDRESSES" long:"tunnel-addresses"`

	TunnelAuthority   string `usage:"The certificate authority, in PEM form, the ingress's certificate has to be signed by. The authority's private key is never needed here." env:"RUNNER_TUNNEL_CA_CERT" long:"tunnel-ca-cert"`
	TunnelCertificate string `usage:"The certificate, in PEM form, this orchestrator proves itself with." env:"RUNNER_TUNNEL_CERT" long:"tunnel-cert"`
	TunnelKey         string `usage:"The private key, in PEM form, for that certificate." env:"RUNNER_TUNNEL_KEY" long:"tunnel-key"`

	TunnelServerName string `usage:"Name the ingress's certificate has to answer for. Without it an orchestrator would hand its credentials to anything the authority ever signed." env:"RUNNER_TUNNEL_SERVER_NAME" long:"tunnel-server-name"`

	TunnelAllowedTargets string `usage:"Addresses this orchestrator will connect a stream to beyond the services it offers, as host:port or host:from-to, separated by commas. Empty offers only named services, which is the only shape an ingress cannot talk an orchestrator out of." env:"RUNNER_TUNNEL_ALLOWED_TARGETS" long:"tunnel-allowed-targets"`

	TunnelMinConnections int           `usage:"How many connections to each ingress are kept open and ready." env:"RUNNER_TUNNEL_MIN_CONNECTIONS" long:"tunnel-min-connections"`
	TunnelMaxConnections int           `usage:"How many connections to each ingress may be open at once. More is how throughput grows: each is its own congestion window." env:"RUNNER_TUNNEL_MAX_CONNECTIONS" long:"tunnel-max-connections"`
	TunnelMaxIdleTime    time.Duration `usage:"How long a connection beyond the fewest may carry nothing before it is let go." env:"RUNNER_TUNNEL_MAX_IDLE_TIME" long:"tunnel-max-idle-time"`

	TunnelMaxStreamsPerSession int `usage:"How many client connections one connection to an ingress will carry before the next is used." env:"RUNNER_TUNNEL_MAX_STREAMS_PER_SESSION" long:"tunnel-max-streams-per-session"`
}

// NewRunnerOrchestrator returns the configuration of the serve-runner-orchestrator
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerOrchestrator() *RunnerOrchestrator {
	return &RunnerOrchestrator{
		Port:                       defaultRunnerOrchestratorPort,
		Runtime:                    defaultRunnerRuntime,
		StateDir:                   defaultRunnerStateDir,
		LauncherSocket:             defaultRunnerLauncherSocket,
		GuestBinary:                defaultRunnerGuestBinary,
		Nameservers:                defaultRunnerNameservers,
		OrchestratorUID:            defaultRunnerOrchestratorUID,
		OrchestratorGID:            defaultRunnerOrchestratorGID,
		TunnelMinConnections:       defaultRunnerTunnelMinConnections,
		TunnelMaxConnections:       defaultRunnerTunnelMaxConnections,
		TunnelMaxIdleTime:          defaultRunnerTunnelMaxIdleTime,
		TunnelMaxStreamsPerSession: defaultTunnelMaxStreamsPerSession,
	}
}

// IngressAddresses is every ingress this orchestrator opens connections to.
//
// The console binds scalars, so the list travels as one comma-separated value
// and is taken apart here — the same way the profiler's headers do.
func (c *RunnerOrchestrator) IngressAddresses() []string {
	return commaSeparated(c.TunnelAddresses)
}

func commaSeparated(value string) []string {
	items := make([]string, 0, 1)

	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); len(item) > 0 {
			items = append(items, item)
		}
	}

	return items
}

// AllowedTargets is what this orchestrator will connect a stream to beyond the
// services it offers by name.
func (c *RunnerOrchestrator) AllowedTargets() ([]tunnel.AddressRule, error) {
	return tunnel.ParseAddressRules(c.TunnelAllowedTargets)
}

// Forwards is the ports this ingress carries arbitrary TCP into the tunnel on.
func (c *RunnerIngress) Forwards() ([]tunnel.Forward, error) {
	return tunnel.ParseForwards(c.ForwardedPorts)
}

// NameserverList is the nameservers a machine that routes out is given.
func (c *RunnerOrchestrator) NameserverList() []string {
	return commaSeparated(c.Nameservers)
}

// RunnerLauncher holds the configuration of the serve-runner-launcher command.
type RunnerLauncher struct {
	// Socket is where the launcher takes orders. Whoever can open it can start
	// machines, so it is made for the orchestrators alone.
	Socket string `usage:"Unix socket the launcher takes orders on. It is made for the orchestrators' uid and gid alone." env:"RUNNER_LAUNCHER_SOCKET" long:"socket"`

	HealthPort int `usage:"Port on 127.0.0.1 the launcher answers its healthcheck on." env:"RUNNER_LAUNCHER_HEALTH_PORT" long:"health-port"`

	StateDir string `usage:"Directory machines are made in, shared with the orchestrators at the same path on both sides." env:"RUNNER_STATE_DIR" long:"state-dir"`

	// Kernel is installed into the state directory when the launcher starts,
	// which is where machines boot it from.
	Kernel string `usage:"Kernel machines boot, installed into the state directory when the launcher starts." env:"RUNNER_KERNEL" long:"kernel"`

	FirecrackerBinary string `usage:"The firecracker a machine runs in." env:"RUNNER_FIRECRACKER_BINARY" long:"firecracker-binary"`

	// OrchestratorUID and OrchestratorGID are who the orchestrators run as.
	// The socket is theirs, and what they write in the state directory; and
	// a machine's directory and sockets are open to their group.
	OrchestratorUID int `usage:"Who the orchestrators run as. The launcher's socket is theirs alone, and so is what they write in the state directory." env:"RUNNER_ORCHESTRATOR_UID" long:"orchestrator-uid"`
	OrchestratorGID int `usage:"The orchestrators' group, which a machine's directory and sockets are open to." env:"RUNNER_ORCHESTRATOR_GID" long:"orchestrator-gid"`

	// MachineFirstUID and MachineUIDs are the users machines run as: each
	// machine is given one of its own, and a group of the same number.
	MachineFirstUID int `usage:"The first of the users machines run as. Each machine runs as one of its own, counting up from this one, with a group of the same number; nothing else may use them." env:"RUNNER_MACHINE_FIRST_UID" long:"machine-first-uid"`
	MachineUIDs     int `usage:"How many users machines may run as, which is also the most machines the launcher runs at once. Zero runs every machine as the launcher itself, which is for development and nothing else." env:"RUNNER_MACHINE_UIDS" long:"machine-uids"`

	NetworkPool string `usage:"Addresses machines' networks are carved out of, a /24 each. They must be the runner's alone." env:"RUNNER_NETWORK_POOL" long:"network-pool"`

	MaxVCPUs     int `usage:"The most CPUs one machine may have." env:"RUNNER_MAX_VCPUS" long:"max-vcpus"`
	MaxMemoryMiB int `usage:"The most memory, in MiB, one machine may have." env:"RUNNER_MAX_MEMORY_MIB" long:"max-memory-mib"`
}

// NewRunnerLauncher returns the configuration of the serve-runner-launcher
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerLauncher() *RunnerLauncher {
	return &RunnerLauncher{
		Socket:            defaultRunnerLauncherSocket,
		HealthPort:        defaultRunnerLauncherHealthPort,
		StateDir:          defaultRunnerStateDir,
		Kernel:            defaultRunnerKernel,
		FirecrackerBinary: defaultRunnerFirecrackerBinary,
		OrchestratorUID:   defaultRunnerOrchestratorUID,
		OrchestratorGID:   defaultRunnerOrchestratorGID,
		MachineFirstUID:   defaultRunnerMachineFirstUID,
		MachineUIDs:       defaultRunnerMachineUIDs,
		NetworkPool:       defaultRunnerNetworkPool,
		MaxVCPUs:          defaultRunnerLauncherMaxVCPUs,
		MaxMemoryMiB:      defaultRunnerLauncherMaxMemoryMiB,
	}
}

// Pool is the addresses machines' networks are carved out of.
func (c *RunnerLauncher) Pool() (*net.IPNet, error) {
	_, pool, err := net.ParseCIDR(c.NetworkPool)
	if err != nil {
		return nil, fmt.Errorf("%q is not a network pool: %w", c.NetworkPool, err)
	}

	return pool, nil
}
