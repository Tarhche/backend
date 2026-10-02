package vm

import (
	"net/url"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

// What vmhost and the orchestrator's microvm driver say to each other: HTTP
// and JSON on a unix socket, shaped like the docker engine's API, so the
// driver reads like the container driver does. Streams — a command's terminal,
// a connection to a task's port — are an HTTP upgrade, as docker's attach is.
//
// Both sides read the shapes from here, so they cannot drift apart.

// DefaultSocket is where vmhost takes requests: a unix socket on the host,
// mode 0660 and the orchestrators' group, so that nothing but an orchestrator
// on the same host can ask it for anything.
const DefaultSocket = "/run/workload-vmhost/vmhost.sock"

// DefaultEndpoint is DefaultSocket as an orchestrator's configuration names it.
const DefaultEndpoint = "unix://" + DefaultSocket

// vmhost's routes, as Go's ServeMux takes them.
const (
	// RouteInfo answers Info: what vmhost is, whether it is healthy, and how
	// much room it has. It is asked once a second, on every node heartbeat,
	// and answered from memory.
	RouteInfo = "GET /v1/info"

	// RoutePrepareImage takes PrepareImage and answers the Image, pulling
	// and converting it if it has to. It is idempotent.
	RoutePrepareImage = "POST /v1/images/prepare"
	RouteImages       = "GET /v1/images"
	RouteDeleteImage  = "DELETE /v1/images/{digest}"

	// RouteEnsureNetwork takes NetworkSpec and answers the Network.
	RouteEnsureNetwork = "PUT /v1/networks/{name}"
	RouteRemoveNetwork = "DELETE /v1/networks/{name}"

	// RouteCreateVM takes a Spec and answers Created. It makes the VM's
	// disks and record and boots nothing, as docker's create starts nothing.
	RouteCreateVM = "POST /v1/vms"

	// RouteVMs answers every VM carrying the label filters given (QueryLabel),
	// and RouteVM one VM.
	RouteVMs = "GET /v1/vms"
	RouteVM  = "GET /v1/vms/{id}"

	// The lifecycle, with docker's meanings. Stop takes QueryTimeout.
	RouteStartVM   = "POST /v1/vms/{id}/start"
	RouteStopVM    = "POST /v1/vms/{id}/stop"
	RouteRestartVM = "POST /v1/vms/{id}/restart"
	RouteKillVM    = "POST /v1/vms/{id}/kill"
	RouteDeleteVM  = "DELETE /v1/vms/{id}"

	// RouteVMLogs answers a VM's output as LogLines, one JSON line each,
	// after QueryAfter or from QuerySince, and with QueryFollow what comes.
	RouteVMLogs = "GET /v1/vms/{id}/logs"

	// RouteVMStats answers Stats.
	RouteVMStats = "GET /v1/vms/{id}/stats"

	// RouteExec takes a guest.Exec and, once the command is running, switches
	// the connection to UpgradeExec, answering its ID in ExecIDHeader. What
	// follows are guest frames (guest.ReadFrame), straight from the agent.
	RouteExec = "POST /v1/vms/{id}/exec"

	// RouteEndExec takes a guest.EndExec and answers a guest.Ended.
	RouteEndExec = "POST /v1/vms/{id}/exec/{exec}/end"

	// RouteDial switches the connection to UpgradeDial and makes it a raw
	// byte stream to the task's port (QueryPort), as task.Dialer is.
	RouteDial = "POST /v1/vms/{id}/dial"
)

// The query parameters of the routes that take them.
const (
	// QueryLabel is a label filter, key=value; it may be given more than
	// once, and a VM has to carry every one.
	QueryLabel = "label"

	// QueryTimeout is how long a task being stopped is given to end on its
	// own, as a Go duration ("10s"). Without it, DefaultStopTimeout.
	QueryTimeout = "timeout"

	// QueryAfter is the last line number a reader of a VM's output has, and
	// QuerySince the time (RFC 3339) from which it wants lines; QueryFollow,
	// set to "1", keeps the answer open for what comes.
	QueryAfter  = "after"
	QuerySince  = "since"
	QueryFollow = "follow"

	// QueryPort is the task's port a dial connects to.
	QueryPort = "port"
)

// The upgrades a stream is carried on. They are the agent's own, since vmhost
// passes the agent's stream through as it is.
const (
	UpgradeExec  = guest.UpgradeExec
	UpgradeDial  = guest.UpgradeDial
	ExecIDHeader = guest.ExecIDHeader
)

// DefaultStopTimeout is how long a task being stopped is given to end on its
// own before it is ended, as docker gives a container.
const DefaultStopTimeout = 10 * time.Second

// The paths of vmhost's routes, for whoever asks them.
const (
	PathInfo         = "/v1/info"
	PathPrepareImage = "/v1/images/prepare"
	PathImages       = "/v1/images"
	PathVMs          = "/v1/vms"
)

// PathImage is one image's path.
func PathImage(digest string) string {
	return PathImages + "/" + url.PathEscape(digest)
}

// PathNetwork is one network's path.
func PathNetwork(name string) string {
	return "/v1/networks/" + url.PathEscape(name)
}

// PathVM is one VM's path.
func PathVM(id string) string {
	return PathVMs + "/" + url.PathEscape(id)
}

// PathVMAction is the path of one thing done to a VM: "start", "stop",
// "restart", "kill", "logs", "stats", "exec" or "dial".
func PathVMAction(id string, action string) string {
	return PathVM(id) + "/" + action
}

// PathEndExec is the path that ends one command running inside a VM.
func PathEndExec(id string, exec string) string {
	return PathVMAction(id, "exec") + "/" + url.PathEscape(exec) + "/end"
}

// Info is what vmhost says about itself.
type Info struct {
	// Version is vmhost's, which is the application's.
	Version string `json:"version"`

	// Hypervisor is the VMM it boots machines with, and HypervisorVersion
	// that VMM's version.
	Hypervisor        string `json:"hypervisor"`
	HypervisorVersion string `json:"hypervisor_version,omitempty"`

	// GuestVersion is the protocol version of the agent new machines boot
	// with (guest.ProtocolVersion).
	GuestVersion string `json:"guest_version"`

	// Architecture is the host's, as Go names it: what images are pulled for.
	Architecture string `json:"architecture"`

	// ProcessMode is where its machines' VMMs run: "systemd", as host units
	// that outlive vmhost, or "child", as its own children, which do not.
	ProcessMode string `json:"process_mode"`

	// Healthy says vmhost can make and boot machines right now; Reason says
	// why not, when it cannot.
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason,omitempty"`

	// Capabilities are what the class vmhost stands behind can do, which the
	// microvm driver offers as they are.
	Capabilities runtime.Capabilities `json:"capabilities"`

	// Capacity is what vmhost may give VMs, and what it has given: its
	// budget, rather than the host's.
	Capacity runtime.Capacity `json:"capacity"`
}

// PrepareImage asks for an image to be made ready to boot.
type PrepareImage struct {
	Image string `json:"image"`
}

// NetworkSpec is a network to ensure.
type NetworkSpec struct {
	Masquerade bool `json:"masquerade"`
}

// Created is the VM a create made.
type Created struct {
	ID string `json:"id"`
}

// LogLine is one line a VM's task wrote, as vmhost keeps it. Lines are
// numbered across every boot of the VM, which the agent's own numbers are not,
// so a reader that saw up to N asks for what came after N however many times
// the VM was started since. At is the guest's, which is what a reader that
// resumes by time compares.
type LogLine struct {
	Seq     uint64    `json:"seq"`
	Stream  string    `json:"stream"`
	At      time.Time `json:"at"`
	Content string    `json:"content"`
}

// Stats is what a VM uses: CPU from its own cgroup on the host, measured
// between two askings as docker measures it; memory as the guest sees it,
// since the host's view of a VM's memory only ever grows; network from its
// taps; disk from its cgroup.
type Stats struct {
	PIDs          uint64  `json:"pids"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	NetworkInput  uint64  `json:"network_input"`
	NetworkOutput uint64  `json:"network_output"`
	BlockInput    uint64  `json:"block_input"`
	BlockOutput   uint64  `json:"block_output"`
}
