package configs

import (
	"net/url"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
)

const (
	defaultWorkloadControlPlanePort   = 80
	defaultWorkloadOrchestratorPort   = 80
	defaultWorkloadIngressPort        = 80
	defaultWorkloadIngressDomain      = "workload.localhost"
	defaultWorkloadMaxLogBytes        = 32 << 20 // 32 MB per task
	defaultWorkloadOrchestratorCpu    = 0.5
	defaultWorkloadOrchestratorMemory = 256 << 20 // 256 MB
	defaultWorkloadOrchestratorDisk   = 256 << 20 // 256 MB

	defaultWorkloadTunnelPort = 81

	defaultWorkloadTunnelMinConnections = 2
	defaultWorkloadTunnelMaxConnections = 4
	defaultWorkloadTunnelMaxIdleTime    = 2 * time.Minute

	defaultTunnelMaxStreamsPerSession       = 256
	defaultTunnelMaxSessionsPerOrchestrator = 8

	// every task ran under sysbox before there were classes, and a platform
	// configured as it always was keeps running them that way.
	defaultWorkloadRuntimes       = string(runtime.Sysbox)
	defaultWorkloadDefaultRuntime = string(runtime.Sysbox)

	// defaultWorkloadRuntimeOutageGrace is how long a class a node says is
	// unhealthy keeps that node's tasks of the class unknown, rather than
	// silent: long enough for a vmhost to be redeployed.
	defaultWorkloadRuntimeOutageGrace = 2 * time.Minute
)

// WorkloadControlPlane holds the configuration of the serve-workload-controlplane command.
type WorkloadControlPlane struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	MaxLogBytes int64 `usage:"How much log one task may keep. Past it, further lines are dropped rather than stored." env:"WORKLOAD_MAX_LOG_BYTES" long:"max-log-bytes"`

	DefaultCpu    float64 `usage:"CPUs a task is limited to when its specification names no limit." env:"WORKLOAD_DEFAULT_CPU" long:"default-cpu"`
	DefaultMemory uint64  `usage:"Memory, in bytes, a task is limited to when its specification names no limit." env:"WORKLOAD_DEFAULT_MEMORY" long:"default-memory"`
	DefaultDisk   uint64  `usage:"Disk, in bytes, a task is limited to when its specification names no limit." env:"WORKLOAD_DEFAULT_DISK" long:"default-disk"`

	Runtimes           string        `usage:"Runtime classes a task may ask for, separated by commas. Taking one away turns new tasks of it away; those already running carry on." env:"WORKLOAD_RUNTIMES" long:"runtimes"`
	DefaultRuntime     string        `usage:"Runtime class a task that names none is run with. It has to be one of the classes a task may ask for." env:"WORKLOAD_DEFAULT_RUNTIME" long:"default-runtime"`
	RuntimeOutageGrace time.Duration `usage:"How long a node's tasks of a class the node says is unhealthy are taken for unknown rather than lost, before they are asked for again elsewhere." env:"WORKLOAD_RUNTIME_OUTAGE_GRACE" long:"runtime-outage-grace"`
}

// NewWorkloadControlPlane returns the configuration of the serve-workload-controlplane
// command, holding the defaults it runs with until the console overrides them.
func NewWorkloadControlPlane() *WorkloadControlPlane {
	return &WorkloadControlPlane{
		Port:          defaultWorkloadControlPlanePort,
		MaxLogBytes:   defaultWorkloadMaxLogBytes,
		DefaultCpu:    defaultWorkloadOrchestratorCpu,
		DefaultMemory: defaultWorkloadOrchestratorMemory,
		DefaultDisk:   defaultWorkloadOrchestratorDisk,

		Runtimes:           defaultWorkloadRuntimes,
		DefaultRuntime:     defaultWorkloadDefaultRuntime,
		RuntimeOutageGrace: defaultWorkloadRuntimeOutageGrace,
	}
}

// AllowedRuntimes is every class a task may ask for.
func (c *WorkloadControlPlane) AllowedRuntimes() ([]runtime.Class, error) {
	return runtime.ParseClasses(c.Runtimes)
}

// WorkloadIngress holds the configuration of the serve-workload-ingress command.
type WorkloadIngress struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	Domain string `usage:"Domain a task's exposed ports are served on, without a leading dot. A request to a hostname under it is routed to the task the hostname names." env:"WORKLOAD_INGRESS_DOMAIN" long:"domain"`

	TunnelPort int `usage:"Port the orchestrators open their connections to. It carries nothing but them, so it is not the port requests arrive on." env:"WORKLOAD_TUNNEL_PORT" long:"tunnel-port"`

	TunnelAuthority   string `usage:"The certificate authority, in PEM form, an orchestrator's own certificate has to be signed by. The authority's private key is never needed here." env:"WORKLOAD_TUNNEL_CA_CERT" long:"tunnel-ca-cert"`
	TunnelCertificate string `usage:"The certificate, in PEM form, the ingress answers with." env:"WORKLOAD_TUNNEL_CERT" long:"tunnel-cert"`
	TunnelKey         string `usage:"The private key, in PEM form, for that certificate." env:"WORKLOAD_TUNNEL_KEY" long:"tunnel-key"`

	TunnelIdentitySuffix       string `usage:"Domain an orchestrator's certificate carries its name under, dropped to leave the name. Empty takes the first subject alternative name whole." env:"WORKLOAD_TUNNEL_IDENTITY_SUFFIX" long:"tunnel-identity-suffix"`
	TunnelAllowedOrchestrators string `usage:"Orchestrators allowed to connect, separated by commas. Empty allows every orchestrator the authority signed for." env:"WORKLOAD_TUNNEL_ALLOWED_ORCHESTRATORS" long:"tunnel-allowed-orchestrators"`

	TunnelMaxStreamsPerSession       int `usage:"How many client connections one of an orchestrator's connections will carry before the next is used. It is a blast radius before it is a capacity: they all end when it does." env:"WORKLOAD_TUNNEL_MAX_STREAMS_PER_SESSION" long:"tunnel-max-streams-per-session"`
	TunnelMaxSessionsPerOrchestrator int `usage:"How many connections one orchestrator may hold here." env:"WORKLOAD_TUNNEL_MAX_SESSIONS_PER_ORCHESTRATOR" long:"tunnel-max-sessions-per-orchestrator"`

	ForwardedPorts string `usage:"Ports to carry arbitrary TCP into the tunnel on, as listen=orchestrator:target, separated by commas — 8022=orchestrator-a:22 reaches port 22 on that orchestrator, 9000=:api reaches the api service on whichever orchestrator the router picks. Nothing is forwarded by default." env:"WORKLOAD_INGRESS_FORWARDS" long:"forward"`
}

// NewWorkloadIngress returns the configuration of the serve-workload-ingress
// command, holding the defaults it runs with until the console overrides them.
func NewWorkloadIngress() *WorkloadIngress {
	return &WorkloadIngress{
		Port:                             defaultWorkloadIngressPort,
		Domain:                           defaultWorkloadIngressDomain,
		TunnelPort:                       defaultWorkloadTunnelPort,
		TunnelMaxStreamsPerSession:       defaultTunnelMaxStreamsPerSession,
		TunnelMaxSessionsPerOrchestrator: defaultTunnelMaxSessionsPerOrchestrator,
	}
}

// AllowedOrchestrators is the orchestrators this ingress will take, or none named at all,
// which allows every orchestrator the authority signed for.
func (c *WorkloadIngress) AllowedOrchestrators() []string {
	return commaSeparated(c.TunnelAllowedOrchestrators)
}

// WorkloadOrchestrator holds the configuration of the serve-workload-orchestrator command.
type WorkloadOrchestrator struct {
	Port int    `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`
	Name string `usage:"specifies the unique name of the orchestrator." env:"WORKLOAD_ORCHESTRATOR_NAME" long:"name" short:"n"`

	DockerHost string `usage:"Docker daemon the tasks are run on. Empty uses the Docker client's own default." env:"DOCKER_HOST" long:"docker-host"`

	// Runtimes is every class this orchestrator offers, and what runs each.
	// Empty is sysbox alone, on DockerHost, which is what an orchestrator
	// did before there were classes.
	Runtimes string `usage:"Runtime classes this orchestrator offers, as class=kind@endpoint separated by commas, with a driver's options as the endpoint's query: sysbox=container@tcp://docker:2375,firecracker=microvm@unix:///run/workload-vmhost/vmhost.sock. Empty offers sysbox alone, on the docker daemon the tasks are run on." env:"WORKLOAD_ORCHESTRATOR_RUNTIMES" long:"runtimes"`

	// PublicKey verifies the tokens the blog signs. An orchestrator never mints one,
	// so it is given the public half and nothing else.
	PublicKey string `usage:"ECDSA public key, in PEM form, the access tokens are verified against. It is the public half of the key the blog signs them with." env:"PUBLIC_KEY" long:"public-key"`

	// AdvertiseHost is where this orchestrator reaches the ports its own tasks
	// publish. It is the docker daemon's host rather than this service's, which
	// are not the same machine when the daemon is a service of its own.
	AdvertiseHost string `usage:"Host this orchestrator reaches its tasks' published ports at, which is the docker daemon's own rather than this one." env:"WORKLOAD_ORCHESTRATOR_ADVERTISE_HOST" long:"advertise-host"`

	TunnelAddresses string `usage:"host:port of every ingress this orchestrator opens connections to, separated by commas. It keeps a pool at each, so it is reachable through all of them." env:"WORKLOAD_TUNNEL_ADDRESSES" long:"tunnel-addresses"`

	TunnelAuthority   string `usage:"The certificate authority, in PEM form, the ingress's certificate has to be signed by. The authority's private key is never needed here." env:"WORKLOAD_TUNNEL_CA_CERT" long:"tunnel-ca-cert"`
	TunnelCertificate string `usage:"The certificate, in PEM form, this orchestrator proves itself with." env:"WORKLOAD_TUNNEL_CERT" long:"tunnel-cert"`
	TunnelKey         string `usage:"The private key, in PEM form, for that certificate." env:"WORKLOAD_TUNNEL_KEY" long:"tunnel-key"`

	TunnelServerName string `usage:"Name the ingress's certificate has to answer for. Without it an orchestrator would hand its credentials to anything the authority ever signed." env:"WORKLOAD_TUNNEL_SERVER_NAME" long:"tunnel-server-name"`

	TunnelAllowedTargets string `usage:"Addresses this orchestrator will connect a stream to beyond the services it offers, as host:port or host:from-to, separated by commas. Empty offers only named services, which is the only shape an ingress cannot talk an orchestrator out of." env:"WORKLOAD_TUNNEL_ALLOWED_TARGETS" long:"tunnel-allowed-targets"`

	TunnelMinConnections int           `usage:"How many connections to each ingress are kept open and ready." env:"WORKLOAD_TUNNEL_MIN_CONNECTIONS" long:"tunnel-min-connections"`
	TunnelMaxConnections int           `usage:"How many connections to each ingress may be open at once. More is how throughput grows: each is its own congestion window." env:"WORKLOAD_TUNNEL_MAX_CONNECTIONS" long:"tunnel-max-connections"`
	TunnelMaxIdleTime    time.Duration `usage:"How long a connection beyond the fewest may carry nothing before it is let go." env:"WORKLOAD_TUNNEL_MAX_IDLE_TIME" long:"tunnel-max-idle-time"`

	TunnelMaxStreamsPerSession int `usage:"How many client connections one connection to an ingress will carry before the next is used." env:"WORKLOAD_TUNNEL_MAX_STREAMS_PER_SESSION" long:"tunnel-max-streams-per-session"`
}

// NewWorkloadOrchestrator returns the configuration of the serve-workload-orchestrator
// command, holding the defaults it runs with until the console overrides them.
func NewWorkloadOrchestrator() *WorkloadOrchestrator {
	return &WorkloadOrchestrator{
		Port:                       defaultWorkloadOrchestratorPort,
		TunnelMinConnections:       defaultWorkloadTunnelMinConnections,
		TunnelMaxConnections:       defaultWorkloadTunnelMaxConnections,
		TunnelMaxIdleTime:          defaultWorkloadTunnelMaxIdleTime,
		TunnelMaxStreamsPerSession: defaultTunnelMaxStreamsPerSession,
	}
}

// IngressAddresses is every ingress this orchestrator opens connections to.
//
// The console binds scalars, so the list travels as one comma-separated value
// and is taken apart here — the same way the profiler's headers do.
func (c *WorkloadOrchestrator) IngressAddresses() []string {
	return commaSeparated(c.TunnelAddresses)
}

// RuntimeSpecs is every class this orchestrator offers.
//
// Naming none offers sysbox alone, on DockerHost: an orchestrator configured as
// it always was runs as it always did. A container driver whose options say
// nothing about where its daemon publishes ports is told AdvertiseHost, which
// is where that used to be said.
func (c *WorkloadOrchestrator) RuntimeSpecs() ([]driver.Spec, error) {
	specs, err := driver.ParseSpecs(c.Runtimes)
	if err != nil {
		return nil, err
	}

	if len(specs) == 0 {
		specs = []driver.Spec{{
			Class:    runtime.Sysbox,
			Kind:     driver.KindContainer,
			Endpoint: c.DockerHost,
			Options:  url.Values{},
		}}
	}

	for i := range specs {
		if specs[i].Kind == driver.KindContainer && len(specs[i].Option(driver.OptionAdvertiseHost)) == 0 && len(c.AdvertiseHost) > 0 {
			specs[i].Options.Set(driver.OptionAdvertiseHost, c.AdvertiseHost)
		}
	}

	return specs, nil
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
func (c *WorkloadOrchestrator) AllowedTargets() ([]tunnel.AddressRule, error) {
	return tunnel.ParseAddressRules(c.TunnelAllowedTargets)
}

// Forwards is the ports this ingress carries arbitrary TCP into the tunnel on.
func (c *WorkloadIngress) Forwards() ([]tunnel.Forward, error) {
	return tunnel.ParseForwards(c.ForwardedPorts)
}
