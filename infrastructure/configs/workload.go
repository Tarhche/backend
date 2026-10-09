package configs

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
)

const (
	defaultWorkloadControlPlanePort = 80
	defaultWorkloadOrchestratorPort = 80
	defaultWorkloadIngressPort      = 80
	defaultWorkloadIngressDomain    = "workload.localhost"
	defaultWorkloadMaxLogBytes      = 32 << 20 // 32 MB per task

	// as long as the control plane waits before it takes a resource its node
	// goes on beating without a word of to be gone from it.
	defaultWorkloadIngressResourceSilentAfter = 30 * time.Second

	defaultWorkloadTunnelPort = 81

	defaultWorkloadTunnelMinConnections = 2
	defaultWorkloadTunnelMaxConnections = 4
	defaultWorkloadTunnelMaxIdleTime    = 2 * time.Minute

	defaultTunnelMaxStreamsPerSession       = 256
	defaultTunnelMaxSessionsPerOrchestrator = 8
)

// WorkloadControlPlane holds the configuration of the serve-workload-controlplane command.
//
// Sizes are bytes, like every size in the workload, and CPUs are whole vCPUs.
type WorkloadControlPlane struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	MaxLogBytes int64 `usage:"How much log one task may keep. Past it, further lines are dropped rather than stored." env:"WORKLOAD_MAX_LOG_BYTES" long:"max-log-bytes"`

	VMDefaultImage string `usage:"Image a machine VM boots from when it names none." env:"WORKLOAD_VM_DEFAULT_IMAGE" long:"vm-default-image"`
	VMDockerImage  string `usage:"Image every Docker VM boots from: a docker-in-docker image, the same one the vmhosts are given. A VM whose image is it, or another tag of it, is a Docker VM." env:"WORKLOAD_VM_DOCKER_IMAGE" long:"vm-docker-image"`

	// the least a VM may be given, which is more for a Docker VM: dockerd and
	// the images it pulls need room before anything runs in it.
	VMMinMemory       uint64 `usage:"Least memory, in bytes, a machine VM may be given." env:"WORKLOAD_VM_MIN_MEMORY" long:"vm-min-memory"`
	VMMinDisk         uint64 `usage:"Least disk, in bytes, a machine VM may be given." env:"WORKLOAD_VM_MIN_DISK" long:"vm-min-disk"`
	VMDockerMinMemory uint64 `usage:"Least memory, in bytes, a Docker VM may be given." env:"WORKLOAD_VM_DOCKER_MIN_MEMORY" long:"vm-docker-min-memory"`
	VMDockerMinDisk   uint64 `usage:"Least disk, in bytes, a Docker VM may be given." env:"WORKLOAD_VM_DOCKER_MIN_DISK" long:"vm-docker-min-disk"`

	// the most one VM may be given.
	VMMaxCPUs   uint   `usage:"Most vCPUs one VM may be given." env:"WORKLOAD_VM_MAX_CPUS" long:"vm-max-cpus"`
	VMMaxMemory uint64 `usage:"Most memory, in bytes, one VM may be given." env:"WORKLOAD_VM_MAX_MEMORY" long:"vm-max-memory"`
	VMMaxDisk   uint64 `usage:"Most disk, in bytes, one VM may be given." env:"WORKLOAD_VM_MAX_DISK" long:"vm-max-disk"`

	// the most one person may have, across all of their VMs.
	VMUserMaxVMs uint   `usage:"Most VMs one person may have." env:"WORKLOAD_VM_USER_MAX_VMS" long:"vm-user-max-vms"`
	VMUserCPUs   uint   `usage:"Most vCPUs one person's VMs may be given between them." env:"WORKLOAD_VM_USER_CPUS" long:"vm-user-cpus"`
	VMUserMemory uint64 `usage:"Most memory, in bytes, one person's VMs may be given between them." env:"WORKLOAD_VM_USER_MEMORY" long:"vm-user-memory"`
	VMUserDisk   uint64 `usage:"Most disk, in bytes, one person's VMs may be given between them." env:"WORKLOAD_VM_USER_DISK" long:"vm-user-disk"`

	VMMaxLifetime time.Duration `usage:"Longest lifetime a VM may ask for. A VM kept until it is deleted asks for none." env:"WORKLOAD_VM_MAX_LIFETIME" long:"vm-max-lifetime"`

	VMCPUOvercommit float64 `usage:"How many times over a node's vCPUs may be given to VMs. Memory and disk are never given twice." env:"WORKLOAD_VM_CPU_OVERCOMMIT" long:"vm-cpu-overcommit"`

	// the Docker VM made for somebody who adds a container or a stack and has
	// none to put it in.
	VMDockerDefaultCPUs           uint          `usage:"vCPUs a Docker VM made for a container or a stack is given." env:"WORKLOAD_VM_DOCKER_DEFAULT_CPUS" long:"vm-docker-default-cpus"`
	VMDockerDefaultMemory         uint64        `usage:"Memory, in bytes, a Docker VM made for a container or a stack is given." env:"WORKLOAD_VM_DOCKER_DEFAULT_MEMORY" long:"vm-docker-default-memory"`
	VMDockerDefaultDisk           uint64        `usage:"Disk, in bytes, a Docker VM made for a container or a stack is given." env:"WORKLOAD_VM_DOCKER_DEFAULT_DISK" long:"vm-docker-default-disk"`
	VMDockerDefaultPorts          string        `usage:"Ports a Docker VM made for a container or a stack exposes through the ingress, separated by commas." env:"WORKLOAD_VM_DOCKER_DEFAULT_PORTS" long:"vm-docker-default-ports"`
	VMDockerDefaultIngress        string        `usage:"Whether a Docker VM made for a container or a stack is reachable through the ingress: allow or deny." env:"WORKLOAD_VM_DOCKER_DEFAULT_INGRESS" long:"vm-docker-default-ingress"`
	VMDockerDefaultEgress         string        `usage:"Whether a Docker VM made for a container or a stack reaches the internet: allow or deny." env:"WORKLOAD_VM_DOCKER_DEFAULT_EGRESS" long:"vm-docker-default-egress"`
	VMDockerDefaultPersistentDisk bool          `usage:"Whether a Docker VM made for a container or a stack keeps its disk across a stop and a start." env:"WORKLOAD_VM_DOCKER_DEFAULT_PERSISTENT_DISK" long:"vm-docker-default-persistent-disk"`
	VMDockerDefaultLifetime       time.Duration `usage:"Lifetime of a Docker VM made for a container or a stack. Zero keeps it until it is deleted." env:"WORKLOAD_VM_DOCKER_DEFAULT_LIFETIME" long:"vm-docker-default-lifetime"`

	SnapshotUserMax uint `usage:"Most snapshots one person may keep." env:"WORKLOAD_SNAPSHOT_USER_MAX" long:"snapshot-user-max"`

	// the control plane deletes a snapshot's archive itself, so it reaches the
	// bucket the nodes write them to.
	SnapshotStorage WorkloadSnapshotStorage

	NodeRequestTimeout time.Duration `usage:"How long a node is given to answer a request: a kind's query, a VM's log say, or a command for what nobody keeps a record of." env:"WORKLOAD_NODE_REQUEST_TIMEOUT" long:"node-request-timeout"`

	// what the nodes give a command that may pull an image first, which the
	// control plane waits for before it asks for it again: the same two the
	// nodes read.
	DockerReadyTimeout time.Duration `usage:"How long a node waits for a Docker VM's dockerd, while the VM comes up, before a command to it fails: what a command that may pull an image is waited on for first." env:"WORKLOAD_DOCKER_READY_TIMEOUT" long:"docker-ready-timeout"`
	DockerPullTimeout  time.Duration `usage:"How long creating a container, or pulling an image, may take inside a Docker VM: what a command that may pull an image is waited on for, after its dockerd." env:"WORKLOAD_DOCKER_PULL_TIMEOUT" long:"docker-pull-timeout"`
}

// NewWorkloadControlPlane returns the configuration of the serve-workload-controlplane
// command, holding the defaults it runs with until the console overrides them.
func NewWorkloadControlPlane() *WorkloadControlPlane {
	return &WorkloadControlPlane{
		Port:        defaultWorkloadControlPlanePort,
		MaxLogBytes: defaultWorkloadMaxLogBytes,

		VMDefaultImage: defaultWorkloadVMDefaultImage,
		VMDockerImage:  defaultWorkloadVMDockerImage,

		VMMinMemory:       defaultWorkloadVMMinMemory,
		VMMinDisk:         defaultWorkloadVMMinDisk,
		VMDockerMinMemory: defaultWorkloadVMDockerMinMemory,
		VMDockerMinDisk:   defaultWorkloadVMDockerMinDisk,

		VMMaxCPUs:   defaultWorkloadVMMaxCPUs,
		VMMaxMemory: defaultWorkloadVMMaxMemory,
		VMMaxDisk:   defaultWorkloadVMMaxDisk,

		VMUserMaxVMs: defaultWorkloadVMUserMaxVMs,
		VMUserCPUs:   defaultWorkloadVMUserCPUs,
		VMUserMemory: defaultWorkloadVMUserMemory,
		VMUserDisk:   defaultWorkloadVMUserDisk,

		VMMaxLifetime:   defaultWorkloadVMMaxLifetime,
		VMCPUOvercommit: defaultWorkloadVMCPUOvercommit,

		VMDockerDefaultCPUs:           defaultWorkloadVMDockerDefaultCPUs,
		VMDockerDefaultMemory:         defaultWorkloadVMDockerDefaultMemory,
		VMDockerDefaultDisk:           defaultWorkloadVMDockerDefaultDisk,
		VMDockerDefaultPorts:          defaultWorkloadVMDockerDefaultPorts,
		VMDockerDefaultIngress:        defaultWorkloadVMDockerDefaultIngress,
		VMDockerDefaultEgress:         defaultWorkloadVMDockerDefaultEgress,
		VMDockerDefaultPersistentDisk: defaultWorkloadVMDockerDefaultPersistentDisk,

		SnapshotUserMax: defaultWorkloadSnapshotUserMax,
		SnapshotStorage: newWorkloadSnapshotStorage(),

		NodeRequestTimeout: defaultWorkloadNodeRequestTimeout,
		DockerReadyTimeout: defaultWorkloadDockerReadyTimeout,
		DockerPullTimeout:  defaultWorkloadDockerPullTimeout,
	}
}

// PullTimeout is how long a node may take over a command that may pull an
// image first, as the nodes are configured to give it: the wait for the VM's
// dockerd, and then the pull.
func (c *WorkloadControlPlane) PullTimeout() time.Duration {
	return c.DockerReadyTimeout + c.DockerPullTimeout
}

// DockerDefaultPorts is the ports a Docker VM made for a container or a stack
// exposes, taken apart the way every list setting is.
func (c *WorkloadControlPlane) DockerDefaultPorts() ([]port.Port, error) {
	items := commaSeparated(c.VMDockerDefaultPorts)

	ports := make([]port.Port, len(items))
	for i, item := range items {
		number, err := strconv.ParseUint(item, 10, 16)
		if err != nil || number == 0 {
			return nil, fmt.Errorf("the docker default ports are numbers from 1 to 65535, got %q", item)
		}

		ports[i] = port.Port(number)
	}

	return ports, nil
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

	// where a task or a VM is, is what the heartbeats of the node holding it
	// say, every second, and no heartbeat says one is gone.
	ResourceSilentAfter time.Duration `usage:"How long a task or a VM may go unheard in its node's heartbeats before the ingress forgets which node holds it. A node says it every second." env:"WORKLOAD_INGRESS_RESOURCE_SILENT_AFTER" long:"resource-silent-after"`
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
		ResourceSilentAfter:              defaultWorkloadIngressResourceSilentAfter,
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

	// PublicKey verifies the tokens the blog signs. An orchestrator never mints one,
	// so it is given the public half and nothing else.
	PublicKey string `usage:"ECDSA public key, in PEM form, the access tokens are verified against. It is the public half of the key the blog signs them with." env:"PUBLIC_KEY" long:"public-key"`

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

	// VMHostSocket is where this orchestrator reaches its own vmhost, the
	// engine its VMs run on. The two are a pair and share nothing with any
	// other node.
	VMHostSocket string `usage:"Unix socket this orchestrator's vmhost serves its engine on." env:"WORKLOAD_VMHOST_SOCKET" long:"vmhost-socket"`

	DockerReadyTimeout time.Duration `usage:"How long a Docker VM's dockerd is waited for, while the VM comes up, before a request to it is refused as docker_unavailable." env:"WORKLOAD_DOCKER_READY_TIMEOUT" long:"docker-ready-timeout"`
	DockerPullTimeout  time.Duration `usage:"How long creating a container, or pulling an image, may take inside a Docker VM." env:"WORKLOAD_DOCKER_PULL_TIMEOUT" long:"docker-pull-timeout"`

	// the bucket this node writes its VMs' snapshots to and restores them
	// from.
	SnapshotStorage WorkloadSnapshotStorage

	NodeRequestConcurrency int `usage:"How many of the control plane's requests this node answers at once." env:"WORKLOAD_NODE_REQUEST_CONCURRENCY" long:"node-request-concurrency"`
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

		VMHostSocket:           defaultWorkloadVMHostSocket,
		DockerReadyTimeout:     defaultWorkloadDockerReadyTimeout,
		DockerPullTimeout:      defaultWorkloadDockerPullTimeout,
		SnapshotStorage:        newWorkloadSnapshotStorage(),
		NodeRequestConcurrency: defaultWorkloadNodeRequestConcurrency,
	}
}

// PullRequestTimeout is how long a command that may pull an image may take,
// a container made or an image pulled: the wait for the VM's dockerd, and
// then the pull.
func (c *WorkloadOrchestrator) PullRequestTimeout() time.Duration {
	return c.DockerReadyTimeout + c.DockerPullTimeout
}

// IngressAddresses is every ingress this orchestrator opens connections to.
//
// The console binds scalars, so the list travels as one comma-separated value
// and is taken apart here — the same way the profiler's headers do.
func (c *WorkloadOrchestrator) IngressAddresses() []string {
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
func (c *WorkloadOrchestrator) AllowedTargets() ([]tunnel.AddressRule, error) {
	return tunnel.ParseAddressRules(c.TunnelAllowedTargets)
}

// Forwards is the ports this ingress carries arbitrary TCP into the tunnel on.
func (c *WorkloadIngress) Forwards() ([]tunnel.Forward, error) {
	return tunnel.ParseForwards(c.ForwardedPorts)
}
