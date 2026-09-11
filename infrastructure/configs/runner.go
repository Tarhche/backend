package configs

import (
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/tunnel"
)

const (
	defaultRunnerManagerPort   = 80
	defaultRunnerWorkerPort    = 80
	defaultRunnerIngressPort   = 80
	defaultRunnerIngressDomain = "runner.localhost"
	defaultRunnerIngressURL    = "http://runner-ingress:80"
	defaultRunnerMaxLogBytes   = 32 << 20 // 32 MB per container
	defaultRunnerWorkerCpu     = 0.5
	defaultRunnerWorkerMemory  = 256 << 20 // 256 MB
	defaultRunnerWorkerDisk    = 1 << 30   // 1 GB

	defaultRunnerTunnelPort = 81

	defaultRunnerTunnelMinConnections = 2
	defaultRunnerTunnelMaxConnections = 4
	defaultRunnerTunnelMaxIdleTime    = 2 * time.Minute

	defaultTunnelMaxStreamsPerSession = 256
	defaultTunnelMaxSessionsPerWorker = 8
)

// RunnerManager holds the configuration of the serve-runner-manager command.
type RunnerManager struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	IngressURL string `usage:"Where the runner ingress answers. A terminal is proxied through it to the node holding the container." env:"RUNNER_INGRESS_URL" long:"ingress-url"`

	MaxLogBytes int64 `usage:"How much log one container may keep. Past it, further lines are dropped rather than stored." env:"RUNNER_MAX_LOG_BYTES" long:"max-log-bytes"`

	DefaultCpu    float64 `usage:"CPUs a container is limited to when its specification names no limit." env:"RUNNER_DEFAULT_CPU" long:"default-cpu"`
	DefaultMemory uint64  `usage:"Memory, in bytes, a container is limited to when its specification names no limit." env:"RUNNER_DEFAULT_MEMORY" long:"default-memory"`
	DefaultDisk   uint64  `usage:"Disk, in bytes, a container is limited to when its specification names no limit." env:"RUNNER_DEFAULT_DISK" long:"default-disk"`
}

// NewRunnerManager returns the configuration of the serve-runner-manager
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerManager() *RunnerManager {
	return &RunnerManager{
		Port:          defaultRunnerManagerPort,
		IngressURL:    defaultRunnerIngressURL,
		MaxLogBytes:   defaultRunnerMaxLogBytes,
		DefaultCpu:    defaultRunnerWorkerCpu,
		DefaultMemory: defaultRunnerWorkerMemory,
		DefaultDisk:   defaultRunnerWorkerDisk,
	}
}

// RunnerIngress holds the configuration of the serve-runner-ingress command.
type RunnerIngress struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	Domain string `usage:"Domain a container's exposed ports are served on, without a leading dot. A request to a hostname under it is routed to the container the hostname names." env:"RUNNER_INGRESS_DOMAIN" long:"domain"`

	TunnelPort int `usage:"Port the workers open their connections to. It carries nothing but them, so it is not the port requests arrive on." env:"RUNNER_TUNNEL_PORT" long:"tunnel-port"`

	TunnelAuthority   string `usage:"Path of the certificate authority a worker's own certificate has to be signed by. The authority's private key is never needed here." env:"RUNNER_TUNNEL_CA_CERT" long:"tunnel-ca-cert"`
	TunnelCertificate string `usage:"Path of the certificate the ingress answers with." env:"RUNNER_TUNNEL_CERT" long:"tunnel-cert"`
	TunnelKey         string `usage:"Path of the private key for that certificate." env:"RUNNER_TUNNEL_KEY" long:"tunnel-key"`

	TunnelIdentitySuffix string `usage:"Domain a worker's certificate carries its name under, dropped to leave the name. Empty takes the first subject alternative name whole." env:"RUNNER_TUNNEL_IDENTITY_SUFFIX" long:"tunnel-identity-suffix"`
	TunnelAllowedWorkers string `usage:"Workers allowed to connect, separated by commas. Empty allows every worker the authority signed for." env:"RUNNER_TUNNEL_ALLOWED_WORKERS" long:"tunnel-allowed-workers"`

	TunnelMaxStreamsPerSession int `usage:"How many client connections one of a worker's connections will carry before the next is used. It is a blast radius before it is a capacity: they all end when it does." env:"RUNNER_TUNNEL_MAX_STREAMS_PER_SESSION" long:"tunnel-max-streams-per-session"`
	TunnelMaxSessionsPerWorker int `usage:"How many connections one worker may hold here." env:"RUNNER_TUNNEL_MAX_SESSIONS_PER_WORKER" long:"tunnel-max-sessions-per-worker"`

	ForwardedPorts string `usage:"Ports to carry arbitrary TCP into the tunnel on, as listen=worker:target, separated by commas — 8022=worker-a:22 reaches port 22 on that worker, 9000=:api reaches the api service on whichever worker the router picks. Nothing is forwarded by default." env:"RUNNER_INGRESS_FORWARDS" long:"forward"`
}

// NewRunnerIngress returns the configuration of the serve-runner-ingress
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerIngress() *RunnerIngress {
	return &RunnerIngress{
		Port:                       defaultRunnerIngressPort,
		Domain:                     defaultRunnerIngressDomain,
		TunnelPort:                 defaultRunnerTunnelPort,
		TunnelMaxStreamsPerSession: defaultTunnelMaxStreamsPerSession,
		TunnelMaxSessionsPerWorker: defaultTunnelMaxSessionsPerWorker,
	}
}

// AllowedWorkers is the workers this ingress will take, or none named at all,
// which allows every worker the authority signed for.
func (c *RunnerIngress) AllowedWorkers() []string {
	return commaSeparated(c.TunnelAllowedWorkers)
}

// RunnerWorker holds the configuration of the serve-runner-worker command.
type RunnerWorker struct {
	Port int    `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`
	Name string `usage:"specifies the unique name of the worker." env:"RUNNER_WORKER_NAME" long:"name" short:"n"`

	DockerHost string `usage:"Docker daemon the tasks are run on. Empty uses the Docker client's own default." env:"DOCKER_HOST" long:"docker-host"`

	// AdvertiseHost is where this worker reaches the ports its own containers
	// publish. It is the docker daemon's host rather than this service's, which
	// are not the same machine when the daemon is a service of its own.
	AdvertiseHost string `usage:"Host this worker reaches its containers' published ports at, which is the docker daemon's own rather than this one." env:"RUNNER_WORKER_ADVERTISE_HOST" long:"advertise-host"`

	TunnelAddresses string `usage:"host:port of every ingress this worker opens connections to, separated by commas. It keeps a pool at each, so it is reachable through all of them." env:"RUNNER_TUNNEL_ADDRESSES" long:"tunnel-addresses"`

	TunnelAuthority   string `usage:"Path of the certificate authority the ingress's certificate has to be signed by. The authority's private key is never needed here." env:"RUNNER_TUNNEL_CA_CERT" long:"tunnel-ca-cert"`
	TunnelCertificate string `usage:"Path of the certificate this worker proves itself with." env:"RUNNER_TUNNEL_CERT" long:"tunnel-cert"`
	TunnelKey         string `usage:"Path of the private key for that certificate." env:"RUNNER_TUNNEL_KEY" long:"tunnel-key"`

	TunnelServerName string `usage:"Name the ingress's certificate has to answer for. Without it a worker would hand its credentials to anything the authority ever signed." env:"RUNNER_TUNNEL_SERVER_NAME" long:"tunnel-server-name"`

	TunnelAllowedTargets string `usage:"Addresses this worker will connect a stream to beyond the services it offers, as host:port or host:from-to, separated by commas. Empty offers only named services, which is the only shape an ingress cannot talk a worker out of." env:"RUNNER_TUNNEL_ALLOWED_TARGETS" long:"tunnel-allowed-targets"`

	TunnelMinConnections int           `usage:"How many connections to each ingress are kept open and ready." env:"RUNNER_TUNNEL_MIN_CONNECTIONS" long:"tunnel-min-connections"`
	TunnelMaxConnections int           `usage:"How many connections to each ingress may be open at once. More is how throughput grows: each is its own congestion window." env:"RUNNER_TUNNEL_MAX_CONNECTIONS" long:"tunnel-max-connections"`
	TunnelMaxIdleTime    time.Duration `usage:"How long a connection beyond the fewest may carry nothing before it is let go." env:"RUNNER_TUNNEL_MAX_IDLE_TIME" long:"tunnel-max-idle-time"`

	TunnelMaxStreamsPerSession int `usage:"How many client connections one connection to an ingress will carry before the next is used." env:"RUNNER_TUNNEL_MAX_STREAMS_PER_SESSION" long:"tunnel-max-streams-per-session"`
}

// NewRunnerWorker returns the configuration of the serve-runner-worker
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerWorker() *RunnerWorker {
	return &RunnerWorker{
		Port:                       defaultRunnerWorkerPort,
		TunnelMinConnections:       defaultRunnerTunnelMinConnections,
		TunnelMaxConnections:       defaultRunnerTunnelMaxConnections,
		TunnelMaxIdleTime:          defaultRunnerTunnelMaxIdleTime,
		TunnelMaxStreamsPerSession: defaultTunnelMaxStreamsPerSession,
	}
}

// IngressAddresses is every ingress this worker opens connections to.
//
// The console binds scalars, so the list travels as one comma-separated value
// and is taken apart here — the same way the profiler's headers do.
func (c *RunnerWorker) IngressAddresses() []string {
	return commaSeparated(c.TunnelAddresses)
}

func commaSeparated(value string) []string {
	items := make([]string, 0, 1)

	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); len(item) > 0 {
			items = append(items, item)
		}
	}

	return items
}

// AllowedTargets is what this worker will connect a stream to beyond the
// services it offers by name.
func (c *RunnerWorker) AllowedTargets() ([]tunnel.AddressRule, error) {
	return tunnel.ParseAddressRules(c.TunnelAllowedTargets)
}

// Forwards is the ports this ingress carries arbitrary TCP into the tunnel on.
func (c *RunnerIngress) Forwards() ([]tunnel.Forward, error) {
	return tunnel.ParseForwards(c.ForwardedPorts)
}
