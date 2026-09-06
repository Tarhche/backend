package configs

import "time"

const (
	defaultRunnerManagerPort = 80
	defaultRunnerWorkerPort  = 80
	defaultRunnerIngressPort = 8090
	defaultRunnerIngressHost = "runner.localhost"

	// what a raw connection to a container is accepted on, and how often the
	// ingress looks for containers to accept them for.
	defaultRunnerIngressPortRange = "30000-32767"
	defaultRunnerIngressPoll      = time.Second
	defaultRunnerMaxLogBytes      = 32 << 20 // 32 MB per container
	defaultRunnerWorkerCpu        = 0.5
	defaultRunnerWorkerMemory     = 256 << 20 // 256 MB
	defaultRunnerWorkerDisk       = 1 << 30   // 1 GB
)

// RunnerIngress holds the configuration of the serve-runner-ingress command.
//
// The ingress serves what the containers themselves serve. It is a service of
// its own so that the manager — which schedules containers and keeps them the
// way they were asked to be — is not what a reader's request has to go through:
// the two do different jobs, and only one of them is worth an outage.
type RunnerIngress struct {
	Port   int    `usage:"Port the containers' own exposed ports are served on. A request there is routed to a container by its hostname." env:"RUNNER_INGRESS_PORT" long:"ingress-port"`
	Domain string `usage:"Domain a container's exposed ports are served on, without a leading dot." env:"RUNNER_INGRESS_DOMAIN" long:"ingress-domain"`

	// PortRange is where a container's ports are forwarded whole, for what
	// does not speak http: "30000-32767", or empty to forward nothing.
	PortRange string `usage:"Range of ports raw connections to containers are accepted on, as \"first-last\". Empty serves http alone." env:"RUNNER_INGRESS_PORT_RANGE" long:"ingress-port-range"`

	// PollInterval is how often the ingress looks at what is running, which is
	// what tells it which ports to listen on.
	PollInterval time.Duration `usage:"How often the ingress looks for containers whose ports it should be forwarding." env:"RUNNER_INGRESS_POLL_INTERVAL" long:"ingress-poll-interval"`
}

// NewRunnerIngress returns the configuration of the serve-runner-ingress
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerIngress() *RunnerIngress {
	return &RunnerIngress{
		Port:         defaultRunnerIngressPort,
		Domain:       defaultRunnerIngressHost,
		PortRange:    defaultRunnerIngressPortRange,
		PollInterval: defaultRunnerIngressPoll,
	}
}

// RunnerManager holds the configuration of the serve-runner-manager command.
type RunnerManager struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	// IngressDomain is what a container's hostnames are built from when the
	// manager reports them. Serving them is the ingress's own business.
	IngressDomain string `usage:"Domain a container's exposed ports are served on, without a leading dot." env:"RUNNER_INGRESS_DOMAIN" long:"ingress-domain"`

	// IngressPortRange is the span of ports the ingress accepts raw
	// connections on. Which container gets which is the manager's to decide,
	// since it is the one that knows what else is running.
	IngressPortRange string `usage:"Range of ports raw connections to containers are accepted on, as \"first-last\". Empty gives containers no such address." env:"RUNNER_INGRESS_PORT_RANGE" long:"ingress-port-range"`

	MaxLogBytes int64 `usage:"How much log one container may keep. Past it, further lines are dropped rather than stored." env:"RUNNER_MAX_LOG_BYTES" long:"max-log-bytes"`

	DefaultCpu    float64 `usage:"CPUs a container is limited to when its specification names no limit." env:"RUNNER_DEFAULT_CPU" long:"default-cpu"`
	DefaultMemory uint64  `usage:"Memory, in bytes, a container is limited to when its specification names no limit." env:"RUNNER_DEFAULT_MEMORY" long:"default-memory"`
	DefaultDisk   uint64  `usage:"Disk, in bytes, a container is limited to when its specification names no limit." env:"RUNNER_DEFAULT_DISK" long:"default-disk"`
}

// NewRunnerManager returns the configuration of the serve-runner-manager
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerManager() *RunnerManager {
	return &RunnerManager{
		Port:             defaultRunnerManagerPort,
		IngressDomain:    defaultRunnerIngressHost,
		IngressPortRange: defaultRunnerIngressPortRange,
		MaxLogBytes:      defaultRunnerMaxLogBytes,
		DefaultCpu:       defaultRunnerWorkerCpu,
		DefaultMemory:    defaultRunnerWorkerMemory,
		DefaultDisk:      defaultRunnerWorkerDisk,
	}
}

// RunnerWorker holds the configuration of the serve-runner-worker command.
type RunnerWorker struct {
	Port int    `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`
	Name string `usage:"specifies the unique name of the worker." env:"RUNNER_WORKER_NAME" long:"name" short:"n"`

	DockerHost string `usage:"Docker daemon the tasks are run on. Empty uses the Docker client's own default." env:"DOCKER_HOST" long:"docker-host"`

	AdvertiseHost string `usage:"Host the containers' published ports can be reached at. This is the docker daemon's own host, which is not always this one." env:"RUNNER_WORKER_ADVERTISE_HOST" long:"advertise-host"`
	APIAddress    string `usage:"host:port this worker's own API is reachable at from inside the cluster, which is where the manager proxies terminals to." env:"RUNNER_WORKER_API_ADDRESS" long:"api-address"`
}

// NewRunnerWorker returns the configuration of the serve-runner-worker
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerWorker() *RunnerWorker {
	return &RunnerWorker{
		Port: defaultRunnerWorkerPort,
	}
}
