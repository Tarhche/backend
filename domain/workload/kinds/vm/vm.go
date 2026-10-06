// Package vm is the vm kind: a virtual machine on one node, declared once for
// every service that runs it (domain/workload/kind).
//
// A VM is one of two flavors, which it is made as and never changes: a
// machine, which boots an operating system image and is somebody's to open a
// terminal in, and a Docker VM, which boots the workload's docker-in-docker
// image and is what containers and stacks live in. A VM lives on one node for
// its whole life, because its disk is there, and its node's engine is what
// runs it (vm.Engine): the kind's node strategy is that engine's, and nothing
// else on the node knows a VM is a record anywhere.
//
// Its spec is what it was asked for, its config of ports, network and
// resources among it; its status is what its node last said of it, a sample
// of what it uses and where its ports are published among it, and the config
// its node last gave the instance it runs, which is how a VM whose ports were
// changed is known to be reconfigured on its node.
//
// Its states are the ones the dashboard has always shown a VM in: created,
// scheduled, starting, running, stopping, stopped, restarting, restoring,
// failed and deleting, and deleted once it is gone. A VM its node no longer
// holds is not running, and is observed stopped, so a VM expected running is
// made again by being started.
package vm

import (
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// Name is the kind's word, and Plural what its routes and its
	// permissions are named by: workload.vms.<verb>.
	Name   = "vm"
	Plural = "vms"
)

// A VM's states. Failed and Deleted are the framework's own.
const (
	// Created is a VM admitted and not yet asked of its node: there is
	// nothing of it anywhere yet.
	Created kind.State = "created"

	// Scheduled is a VM its node has been asked to make and boot.
	Scheduled kind.State = "scheduled"

	Starting kind.State = "starting"
	Running  kind.State = "running"
	Stopping kind.State = "stopping"

	// Stopped is a VM that is down, its disk kept where it lives, and one
	// its node holds nothing of.
	Stopped kind.State = "stopped"

	// Restarting is a VM being stopped and booted again in place, which is
	// also how a change to its ports, network or resources is applied to one
	// that runs.
	Restarting kind.State = "restarting"

	// Restoring is a VM whose disk is being replaced from a snapshot. It keeps
	// its identity: its uuid, its slug and its ports.
	Restoring kind.State = "restoring"

	// Deleting is a VM its node has been asked to remove. Its record goes once
	// its node says it is gone.
	Deleting kind.State = "deleting"

	// Failed is a VM that could not be made what it was asked to be, or whose
	// node fell silent. Its reason says why.
	Failed = kind.Failed

	Deleted = kind.Deleted
)

// A VM's actions.
const (
	// ActionCreate makes a VM admitted and asked of nobody yet: from its image,
	// or from its source's snapshot. It is the workload's own to ask for.
	ActionCreate = "create"

	// ActionStart makes a VM run: booting the instance its node holds, or
	// making it when its node holds none, which is how a VM its node lost is
	// brought back.
	ActionStart = "start"

	ActionStop = "stop"

	// ActionRestart stops a running VM and boots it again in place, and
	// starts one that is not running.
	ActionRestart = "restart"

	// ActionUpdate changes a VM in the control plane: its name and lifetime,
	// which are its record's alone, and its ports, network and resources,
	// which its node is then asked to apply (ActionReconfigure).
	ActionUpdate = "update"

	// ActionReconfigure gives the instance a VM's node holds the ports,
	// network and resources the VM now has, restarting it when the engine has
	// to. It is the workload's own to ask for, once a VM's config was changed.
	ActionReconfigure = "reconfigure"

	// ActionRestore replaces a VM's disk with a snapshot's (RestorePayload).
	ActionRestore = "restore"

	// ActionDelete removes a VM from its node, disk and all. Its snapshots
	// outlive it.
	ActionDelete = "delete"

	// ActionState is what a VM is doing, as its node's engine says;
	// ActionLogs reads its log (LogsPayload) and ActionStats samples what it
	// uses, from its node as they are now.
	ActionState = "state"
	ActionLogs  = "logs"
	ActionStats = "stats"

	// ActionAttach opens a terminal in a running VM, for its owner alone.
	ActionAttach = "attach"
)

// Flavor is what a VM boots into: a machine, or a Docker VM. It is the
// engine's vm.Kind, named apart from the kind a VM is.
type Flavor = vm.Kind

const (
	FlavorMachine Flavor = vm.KindMachine
	FlavorDocker  Flavor = vm.KindDocker
)

// The labels of a VM.
const (
	// LabelFlavor is on every VM's record, its flavor: the Docker VMs are the
	// VMs labelled workload.flavor=docker, however they are listed.
	LabelFlavor = "workload.flavor"

	// LabelDocker marks an instance a node's engine holds as a Docker VM,
	// with "true": it is how a node tells, from its engine alone, which of its
	// VMs have a dockerd it reads, since nothing ever looks inside a VM to
	// find one. Every Docker VM's instance is made with it.
	LabelDocker = "workload.docker"

	// LabelManagedBy names what keeps a VM that is not a record of its own,
	// such as ManagedByCodeRunner: it is on the manifest of one of the code
	// runner's runs, listed among anybody's VMs, and on no VM's record.
	LabelManagedBy = "workload.managed-by"

	// ManagedByCodeRunner is what keeps the VM the code runner runs a snippet
	// in: the snippet's task, read as a VM.
	ManagedByCodeRunner = vm.ManagedByCodeRunner
)

// Limits of what a VM may be asked for, whoever asks.
const (
	// MaxPorts is the most ports a VM may expose through the ingress.
	MaxPorts = 16

	// MaxNameLength keeps a name to something a listing can show.
	MaxNameLength = 100

	// maxPort is the highest port there is.
	maxPort = 65535
)

// Spec is what a VM is asked for as.
type Spec struct {
	Flavor Flavor `json:"flavor"`

	// Image is the OCI reference it boots from: its flavor's own when it
	// names none, and never anything else for a Docker VM. It never changes.
	Image string `json:"image,omitempty"`

	Resources Resources `json:"resources"`

	// Ports are the guest ports the ingress serves, sorted and each once.
	Ports []port.Port `json:"ports"`

	Network Network `json:"network"`

	// PersistentDisk keeps what is written to its disk across a stop and a
	// start. Without it, the disk is as its image left it on every start.
	PersistentDisk bool `json:"persistent_disk,omitempty"`

	// Source is what its disk is made from when it is not its image.
	Source *Source `json:"source,omitempty"`
}

// Config is what of a VM's spec its node applies to the instance it runs,
// and can apply again to one that runs already: its ports, network and
// resources.
func (s Spec) Config() Config {
	return Config{Resources: s.Resources, Ports: slices.Clone(s.Ports), Network: s.Network}
}

// Resources are what a VM is given: whole vCPUs, and bytes of memory and
// disk, as they are everywhere in the workload. Nothing between where they
// are asked for and the engine converts them.
type Resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

// VM is the resources as the engine is given them.
func (r Resources) VM() vm.Resources {
	return vm.Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

// ResourcesOf is what the engine calls a VM's resources, as a spec has them.
func ResourcesOf(r vm.Resources) Resources {
	return Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

// Network is which ways a VM's network is open: allow or deny each way.
// Nothing ever lets one VM reach another, whatever either of them allows.
type Network struct {
	// Ingress allowed makes the VM's ports reachable through the ingress;
	// denied, nothing reaches it at all.
	Ingress vm.Access `json:"ingress"`

	// Egress allowed lets the VM reach the public internet, never private
	// ranges, the host or other VMs; denied, it reaches nothing.
	Egress vm.Access `json:"egress"`
}

// VM is the network as the engine is given it.
func (n Network) VM() vm.Network {
	return vm.Network{Ingress: n.Ingress, Egress: n.Egress}
}

// Source is what a VM's disk is made from when it is not its image.
type Source struct {
	// Snapshot is a snapshot of its owner's, whose disk it is made with: its
	// flavor and image are the snapshot's.
	Snapshot string `json:"snapshot"`
}

// Config is a VM's ports, network and resources: what its node gives the
// instance it runs, and can give again to one that runs already.
type Config struct {
	Resources Resources   `json:"resources"`
	Ports     []port.Port `json:"ports"`
	Network   Network     `json:"network"`
}

// Equal reports whether two configs give a VM the same.
func (c Config) Equal(other Config) bool {
	return c.Resources == other.Resources && c.Network == other.Network && slices.Equal(c.Ports, other.Ports)
}

// Status is what a VM is doing: its state, and what its node last said of
// it beside.
type Status struct {
	kind.Status

	// Stats is the last sample of what it uses, while it runs; nothing
	// otherwise, and nothing until its node has sampled it.
	Stats *Stats `json:"stats"`

	// Endpoints are where its node reaches each of its published ports.
	Endpoints []Endpoint `json:"endpoints"`

	// StartedAt is when it last came up.
	StartedAt time.Time `json:"started_at,omitzero"`

	// Applied is the config its node last gave the instance it runs, which it
	// says whenever it makes the instance or reconfigures it, and nothing else
	// does: a VM whose spec says otherwise is to be reconfigured. Nothing is a
	// VM that was never made, whose instance has its spec once it is.
	Applied *Config `json:"applied,omitempty"`

	// RestoredAt is when its disk was last replaced from a snapshot, which
	// its node says when it restores it and nothing else does: what lives in
	// it is what that disk holds from then on, and what is first found there
	// labelled as the platform's is taken in again rather than taken for
	// somebody's own.
	RestoredAt time.Time `json:"restored_at,omitzero"`
}

// Stats is one sample of what a VM uses.
type Stats struct {
	// CPUPercent is how busy the VM kept the vCPUs it was given, as a share of
	// all of them together: 0 is idle and 100 is every one of them busy,
	// however many it has. It means the same from the engine to the
	// dashboard, and nothing on the way converts it.
	CPUPercent float64 `json:"cpu_percent"`

	// Memory and disk are bytes; the network counters are bytes received and
	// sent since the VM started.
	MemoryUsed  uint64 `json:"memory_used"`
	MemoryLimit uint64 `json:"memory_limit"`
	DiskUsed    uint64 `json:"disk_used"`
	DiskTotal   uint64 `json:"disk_total"`
	NetworkRx   uint64 `json:"network_rx"`
	NetworkTx   uint64 `json:"network_tx"`

	SampledAt time.Time `json:"sampled_at"`
}

// StatsOf is a sample the engine took, as a status has it, its CPU percent
// held to 0 to 100.
func StatsOf(s vm.Stats) *Stats {
	return &Stats{
		CPUPercent:  min(max(s.CPUPercent, 0), 100),
		MemoryUsed:  s.MemoryUsed,
		MemoryLimit: s.MemoryLimit,
		DiskUsed:    s.DiskUsed,
		DiskTotal:   s.DiskTotal,
		NetworkRx:   s.NetworkRx,
		NetworkTx:   s.NetworkTx,
		SampledAt:   s.SampledAt,
	}
}

// VM is the sample as the engine took it. Nothing is no sample.
func (s *Stats) VM() vm.Stats {
	if s == nil {
		return vm.Stats{}
	}

	return vm.Stats{
		CPUPercent:  s.CPUPercent,
		MemoryUsed:  s.MemoryUsed,
		MemoryLimit: s.MemoryLimit,
		DiskUsed:    s.DiskUsed,
		DiskTotal:   s.DiskTotal,
		NetworkRx:   s.NetworkRx,
		NetworkTx:   s.NetworkTx,
		SampledAt:   s.SampledAt,
	}
}

// Endpoint is a guest port of a VM and the address its node reaches it at.
type Endpoint struct {
	Port    port.Port `json:"port"`
	Address string    `json:"address"`
}

// EndpointsOf are an instance's endpoints, as a status has them.
func EndpointsOf(endpoints []vm.Endpoint) []Endpoint {
	of := make([]Endpoint, len(endpoints))
	for i, e := range endpoints {
		of[i] = Endpoint{Port: e.Port, Address: e.Address}
	}

	return of
}

// UpdatePayload is what changes about a VM: anything it leaves out stays as
// it is. Its name and lifetime are its record's alone; its ports, network and
// resources are its node's to apply, which restarts it when the engine has
// to. Its flavor and its image never change, and its disk only grows.
//
// What is wrong with it is said under the fields the dashboard asks with.
type UpdatePayload struct {
	Name *string `json:"name,omitempty"`

	// Lifetime counts again from now, in nanoseconds as every duration
	// between the workload's services is; zero keeps the VM until it is
	// deleted.
	Lifetime *time.Duration `json:"lifetime,omitempty"`

	Ports *[]port.Port `json:"ports,omitempty"`

	// Network changes either way it names; a way it leaves empty stays.
	Network *Network `json:"network,omitempty"`

	Resources *Resources `json:"resources,omitempty"`
}

var _ domain.Validatable = &UpdatePayload{}

func (p *UpdatePayload) Validate() domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	if p.Name != nil {
		if code, ok := ValidateName(*p.Name); !ok {
			invalid["name"] = code
		}
	}

	if p.Lifetime != nil && *p.Lifetime < 0 {
		invalid["lifetime_seconds"] = "invalid_lifetime"
	}

	if p.Ports != nil {
		if code, ok := ValidatePorts(*p.Ports); !ok {
			invalid["ports"] = code
		}
	}

	if p.Network != nil {
		for field, access := range map[string]vm.Access{"network.ingress": p.Network.Ingress, "network.egress": p.Network.Egress} {
			if len(access) > 0 && !access.IsValid() {
				invalid[field] = "invalid_access"
			}
		}
	}

	return invalid
}

// Changes reports whether it changes what a VM's node applies: its ports,
// network or resources.
func (p UpdatePayload) Changes() bool {
	return p.Ports != nil || p.Network != nil || p.Resources != nil
}

// RestorePayload is the snapshot a VM's disk is restored from: one of its
// owner's, of its flavor, ready, and no larger than its disk.
type RestorePayload struct {
	SnapshotUUID string `json:"snapshot_uuid"`
}

var _ domain.Validatable = &RestorePayload{}

func (p *RestorePayload) Validate() domain.ValidationErrors {
	if len(strings.TrimSpace(p.SnapshotUUID)) == 0 {
		return domain.ValidationErrors{"snapshot_uuid": "required_field"}
	}

	return nil
}

// LogsPayload narrows what is read of a VM's log.
type LogsPayload struct {
	// Since leaves out the lines written before it; nothing is from the
	// start.
	Since time.Time `json:"since,omitzero"`

	// Tail keeps only the last lines; nothing is as many as an answer
	// carries.
	Tail uint `json:"tail,omitempty"`
}

var _ domain.Validatable = &LogsPayload{}

// Validate says nothing is wrong: any part of a log can be read.
func (p *LogsPayload) Validate() domain.ValidationErrors {
	return nil
}

// Logs are the lines of a VM's log a query read, oldest first, and whether
// there were more of them than an answer carries.
type Logs struct {
	Lines     []LogLine `json:"lines"`
	Truncated bool      `json:"truncated,omitempty"`
}

// LogLine is one line of a VM's log.
type LogLine struct {
	At time.Time `json:"at"`

	// Source is where it came from: one of vm's LogSource values.
	Source string `json:"source"`

	Line string `json:"line"`
}

// LogLineOf is one line of a log, as the engine read it.
func LogLineOf(line vm.LogLine) LogLine {
	return LogLine{At: line.At, Source: line.Source, Line: line.Line}
}

// VM is the line as the engine reads one.
func (l LogLine) VM() vm.LogLine {
	return vm.LogLine{At: l.At, Source: l.Source, Line: l.Line}
}

// ValidateName checks a VM's name: something, and something a listing can
// show.
func ValidateName(name string) (string, bool) {
	switch trimmed := strings.TrimSpace(name); {
	case len(trimmed) == 0:
		return "required_field", false
	case len(trimmed) > MaxNameLength:
		return "invalid_name", false
	}

	return "", true
}

// ValidatePorts checks the ports a VM exposes: ports there are, and no more
// of them than a VM may have. The same port twice is the same port, and is
// not refused for it.
func ValidatePorts(ports []port.Port) (string, bool) {
	if len(ports) > MaxPorts {
		return "too_many_ports", false
	}

	for _, p := range ports {
		if p == 0 || p > maxPort {
			return "invalid_port", false
		}
	}

	return "", true
}

// Normalized is ports sorted, each once, and none rather than nothing.
func Normalized(ports []port.Port) []port.Port {
	sorted := slices.Clone(ports)
	slices.Sort(sorted)

	sorted = slices.Compact(sorted)
	if sorted == nil {
		sorted = []port.Port{}
	}

	return sorted
}

// VM is a VM, as its strategies are handed one.
type VM = kind.Resource[Spec, Status]

// Descriptor is the vm kind, a new one every time.
func Descriptor() kind.Descriptor {
	command := func(name string, allowedIn []kind.State, desires kind.State, permission string, payload kind.Codec) kind.Action {
		return kind.Action{Name: name, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: allowedIn, Desires: desires, Permission: permission, Payload: payload}
	}

	waiting := func(a kind.Action) kind.Action {
		a.Waits = true

		return a
	}

	query := func(name string, allowedIn []kind.State, permission string, payload kind.Codec) kind.Action {
		return kind.Action{Name: name, Runs: kind.OnNode, Mode: kind.ModeQuery, AllowedIn: allowedIn, Permission: permission, Payload: payload}
	}

	everyBut := func(but ...kind.State) []kind.State {
		return slices.DeleteFunc(Machine().States, func(s kind.State) bool { return slices.Contains(but, s) })
	}

	return kind.Descriptor{
		Name:      Name,
		Plural:    Plural,
		StateBy:   kind.OnNode,
		Endpoints: true,
		Machine:   Machine(),
		Actions: []kind.Action{
			{Name: ActionCreate, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Created}, Desires: Running, Internal: true, Timeout: kind.TimeoutTransfer, Payload: kind.NoPayload},
			waiting(command(ActionStart, []kind.State{Created, Stopped, Failed}, Running, "manage", kind.NoPayload)),
			waiting(command(ActionStop, []kind.State{Created, Running, Failed}, Stopped, "manage", kind.NoPayload)),
			waiting(command(ActionRestart, []kind.State{Created, Running, Stopped, Failed}, Running, "manage", kind.NoPayload)),
			{Name: ActionUpdate, Runs: kind.OnControlPlane, Mode: kind.ModeCommand, AllowedIn: everyBut(Deleting, Deleted), Permission: "update", Payload: kind.Payload[UpdatePayload]()},
			{Name: ActionReconfigure, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Running, Stopped, Failed}, Internal: true, Payload: kind.NoPayload},
			{Name: ActionRestore, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Running, Stopped, Failed}, Permission: "manage", Restores: true, Timeout: kind.TimeoutTransfer, Payload: kind.Payload[RestorePayload]()},
			command(ActionDelete, nil, Deleted, "delete", kind.NoPayload),
			query(ActionState, nil, "show", kind.NoPayload),
			query(ActionLogs, []kind.State{Starting, Running, Stopping, Stopped, Restarting, Restoring, Failed}, "logs", kind.Payload[LogsPayload]()),
			query(ActionStats, []kind.State{Running}, "show", kind.NoPayload),
			{Name: ActionAttach, Runs: kind.OnNode, Mode: kind.ModeStream, AllowedIn: []kind.State{Running}, Permission: "attach", Payload: kind.NoPayload},
		},
	}
}

// Machine is a VM's states and the moves between them.
//
// A command is waited on in flight until its own answer, or its node, says
// where it got: scheduled, starting and restarting end in running, stopping
// in stopped, restoring in running or stopped, and deleting in the VM being
// gone. While a node carries a command out on a VM, it says the VM is still
// in flight, so that what the VM is doing halfway through is not taken for
// what the command came to. A create, a restart and a restore are over only
// once they are answered: a VM runs before a restart or a restore as it does
// after it, so its node seeing it run says nothing of whether the command was
// carried out yet; and what a create gave the VM, its ports, network and
// resources (Status.Applied), is in its answer alone, which a heartbeat
// seeing the VM run first would leave nobody waiting for.
//
// At rest, a VM is what its node says: running, stopped or failed. One its
// node holds nothing of is not running: a running VM its node lost is
// observed stopped, and is started again, which makes it, when it is
// expected running. A failed VM its node holds nothing of stays failed, and
// keeps saying why.
func Machine() kind.Machine {
	transitions := []kind.Transition{
		{From: Created, On: kind.OnAction(ActionCreate), To: Scheduled},
		{From: Running, On: kind.OnAction(ActionReconfigure), To: Restarting},
		{From: kind.Any, On: kind.OnAction(ActionDelete), To: Deleting},
		{From: Deleting, On: kind.OnObserved(kind.Missing), To: Deleted},
		{From: Running, On: kind.OnObserved(kind.Missing), To: Stopped},
		{From: kind.Any, On: kind.OnObserved(Failed), To: Failed},
	}

	asked := func(action string, to kind.State, from ...kind.State) {
		for _, state := range from {
			transitions = append(transitions, kind.Transition{From: state, On: kind.OnAction(action), To: to})
		}
	}

	asked(ActionStart, Starting, Created, Stopped, Failed)
	asked(ActionStop, Stopping, Created, Running)
	asked(ActionRestart, Restarting, Running)
	asked(ActionRestart, Starting, Created, Stopped, Failed)
	asked(ActionRestore, Restoring, Running, Stopped, Failed)

	arrives := func(from kind.State, at ...kind.State) {
		for _, state := range at {
			transitions = append(transitions, kind.Transition{From: from, On: kind.OnObserved(state), To: state})
		}
	}

	arrives(Scheduled, Running)
	arrives(Starting, Running)
	arrives(Restarting, Running)
	arrives(Stopping, Stopped)
	arrives(Restoring, Running, Stopped)

	return kind.Machine{
		Initial:     Created,
		States:      []kind.State{Created, Scheduled, Starting, Running, Stopping, Stopped, Restarting, Restoring, Failed, Deleting, Deleted},
		Transitions: transitions,
		Terminal:    []kind.State{Stopped, Failed, Deleted},
		InFlight:    []kind.State{Scheduled, Starting, Stopping, Restarting, Restoring, Deleting},
		Answered:    []kind.State{Scheduled, Restarting, Restoring},
	}
}

// DockerVM reports whether a VM is a Docker VM, which only its flavor says.
func DockerVM(v VM) bool {
	return v.Spec.Flavor == FlavorDocker
}

// Up reports whether a VM's dockerd can be asked anything, now or once it is
// up: a Docker VM, placed on a node, running or on its way up.
func Up(v VM) bool {
	if !DockerVM(v) || len(v.Metadata.Node) == 0 {
		return false
	}

	switch v.Status.State {
	case Scheduled, Starting, Restarting, Running:
		return true
	}

	return false
}
