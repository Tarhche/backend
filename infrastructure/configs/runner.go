package configs

import (
	"strings"
	"time"
)

const (
	defaultRunnerManagerPort = 80
	defaultRunnerWorkerPort  = 80
	defaultRunnerIngressPort = 80

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
}

// NewRunnerManager returns the configuration of the serve-runner-manager
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerManager() *RunnerManager {
	return &RunnerManager{
		Port: defaultRunnerManagerPort,
	}
}

// RunnerIngress holds the configuration of the serve-runner-ingress command.
type RunnerIngress struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	TunnelPort int `usage:"Port the workers open their connections to. It carries nothing but them, so it is not the port requests arrive on." env:"RUNNER_TUNNEL_PORT" long:"tunnel-port"`

	TunnelPrivateKey     string `usage:"Private key, in PEM form, the ingress proves itself to the workers with. Generate one with generate-private-key." env:"RUNNER_TUNNEL_PRIVATE_KEY" long:"tunnel-private-key"`
	TunnelAuthorizedKeys string `usage:"Public keys, in PEM form, of the workers that may connect, one after another. A worker offering anything else is dropped." env:"RUNNER_TUNNEL_AUTHORIZED_KEYS" long:"tunnel-authorized-keys"`
	TunnelToken          string `usage:"Shared secret a worker proves its name with, inside the connection its key already opened." env:"RUNNER_TUNNEL_TOKEN" long:"tunnel-token"`

	TunnelMaxStreamsPerSession int `usage:"How many client connections one of a worker's connections will carry before the next is used. It is a blast radius before it is a capacity: they all end when it does." env:"RUNNER_TUNNEL_MAX_STREAMS_PER_SESSION" long:"tunnel-max-streams-per-session"`
	TunnelMaxSessionsPerWorker int `usage:"How many connections one worker may hold here." env:"RUNNER_TUNNEL_MAX_SESSIONS_PER_WORKER" long:"tunnel-max-sessions-per-worker"`
}

// NewRunnerIngress returns the configuration of the serve-runner-ingress
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerIngress() *RunnerIngress {
	return &RunnerIngress{
		Port:                       defaultRunnerIngressPort,
		TunnelPort:                 defaultRunnerTunnelPort,
		TunnelMaxStreamsPerSession: defaultTunnelMaxStreamsPerSession,
		TunnelMaxSessionsPerWorker: defaultTunnelMaxSessionsPerWorker,
	}
}

// RunnerWorker holds the configuration of the serve-runner-worker command.
type RunnerWorker struct {
	Port int    `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`
	Name string `usage:"specifies the unique name of the worker." env:"RUNNER_WORKER_NAME" long:"name" short:"n"`

	DockerHost string `usage:"Docker daemon the tasks are run on. Empty uses the Docker client's own default." env:"DOCKER_HOST" long:"docker-host"`

	TunnelAddresses string `usage:"host:port of every ingress this worker opens connections to, separated by commas. It keeps a pool at each, so it is reachable through all of them." env:"RUNNER_TUNNEL_ADDRESSES" long:"tunnel-addresses"`

	TunnelPrivateKey        string `usage:"Private key, in PEM form, this worker proves itself to the ingresses with. Generate one with generate-private-key." env:"RUNNER_TUNNEL_PRIVATE_KEY" long:"tunnel-private-key"`
	TunnelIngressPublicKeys string `usage:"Public keys, in PEM form, of the ingresses this worker will talk to, one after another. An ingress offering anything else is not talked to." env:"RUNNER_TUNNEL_INGRESS_PUBLIC_KEYS" long:"tunnel-ingress-public-keys"`
	TunnelToken             string `usage:"Shared secret this worker proves its name with, inside the connection its key already opened." env:"RUNNER_TUNNEL_TOKEN" long:"tunnel-token"`

	TunnelMaxStreamsPerSession int `usage:"How many client connections one connection to an ingress will carry before the next is used." env:"RUNNER_TUNNEL_MAX_STREAMS_PER_SESSION" long:"tunnel-max-streams-per-session"`

	TunnelMinConnections int           `usage:"How many connections to each ingress are kept open and ready." env:"RUNNER_TUNNEL_MIN_CONNECTIONS" long:"tunnel-min-connections"`
	TunnelMaxConnections int           `usage:"How many connections to each ingress may be open at once. More is how throughput grows: each is its own congestion window." env:"RUNNER_TUNNEL_MAX_CONNECTIONS" long:"tunnel-max-connections"`
	TunnelMaxIdleTime    time.Duration `usage:"How long a connection beyond the fewest may carry nothing before it is let go." env:"RUNNER_TUNNEL_MAX_IDLE_TIME" long:"tunnel-max-idle-time"`
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
	addresses := make([]string, 0, 1)

	for _, address := range strings.Split(c.TunnelAddresses, ",") {
		if address = strings.TrimSpace(address); len(address) > 0 {
			addresses = append(addresses, address)
		}
	}

	return addresses
}
