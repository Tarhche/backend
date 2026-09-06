package configs

const (
	defaultRunnerManagerPort  = 80
	defaultRunnerWorkerPort   = 80
	defaultRunnerIngressPort  = 8090
	defaultRunnerIngressHost  = "runner.localhost"
	defaultRunnerMaxLogBytes  = 32 << 20 // 32 MB per container
	defaultRunnerWorkerCpu    = 0.5
	defaultRunnerWorkerMemory = 256 << 20 // 256 MB
	defaultRunnerWorkerDisk   = 1 << 30   // 1 GB
)

// RunnerManager holds the configuration of the serve-runner-manager command.
type RunnerManager struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	// IngressDomain is what a container's hostnames are built from when the
	// manager reports them. Serving them is the ingress's own business.
	IngressDomain string `usage:"Domain a container's exposed ports are served on, without a leading dot." env:"RUNNER_INGRESS_DOMAIN" long:"ingress-domain"`

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
		IngressDomain: defaultRunnerIngressHost,
		MaxLogBytes:   defaultRunnerMaxLogBytes,
		DefaultCpu:    defaultRunnerWorkerCpu,
		DefaultMemory: defaultRunnerWorkerMemory,
		DefaultDisk:   defaultRunnerWorkerDisk,
	}
}

// RunnerWorker holds the configuration of the serve-runner-worker command.
type RunnerWorker struct {
	Port int    `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`
	Name string `usage:"specifies the unique name of the worker. Empty takes the machine's own name, which is what lets a node be one of many alike." env:"RUNNER_WORKER_NAME" long:"name" short:"n"`

	DockerHost string `usage:"Docker daemon the tasks are run on. Empty uses the Docker client's own default." env:"DOCKER_HOST" long:"docker-host"`

	AdvertiseHost string `usage:"Host the containers' published ports can be reached at. This is the docker daemon's own host, which is not always this one." env:"RUNNER_WORKER_ADVERTISE_HOST" long:"advertise-host"`
	APIAddress    string `usage:"host:port this worker's own API is reachable at from inside the cluster, which is where the manager proxies terminals to. Empty is the machine's own name and this node's port." env:"RUNNER_WORKER_API_ADDRESS" long:"api-address"`

	// A node serves what its own containers serve, and passes on what belongs
	// to another node: http by the name a container answers on, and everything
	// else on a port out of this node's own range.
	IngressPort   int    `usage:"Port the containers' own exposed ports are served on. A request there is routed to a container by its hostname, whichever node is holding it." env:"RUNNER_INGRESS_PORT" long:"ingress-port"`
	IngressDomain string `usage:"Domain a container's exposed ports are served on, without a leading dot." env:"RUNNER_INGRESS_DOMAIN" long:"ingress-domain"`

	PublicPortRange string `usage:"Range of ports a container's own ports are published on, as \"first-last\", so that what does not speak http has an address. Empty lets docker pick, which is an address nobody can predict." env:"RUNNER_WORKER_PUBLIC_PORT_RANGE" long:"public-port-range"`
	PublicHost      string `usage:"Host somebody outside reaches this node's forwarded ports at. Empty uses the advertised host." env:"RUNNER_WORKER_PUBLIC_HOST" long:"public-host"`
}

// NewRunnerWorker returns the configuration of the serve-runner-worker
// command, holding the defaults it runs with until the console overrides them.
func NewRunnerWorker() *RunnerWorker {
	return &RunnerWorker{
		Port:          defaultRunnerWorkerPort,
		IngressPort:   defaultRunnerIngressPort,
		IngressDomain: defaultRunnerIngressHost,
	}
}
