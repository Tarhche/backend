// Package container is the container kind: a container in a Docker VM,
// stored and reconciled like a Kubernetes pod, declared once for every
// service that runs it (domain/workload/kind).
//
// It is a building block of its Docker VM (domain/workload/kinds/blocks): it
// lives in the VM, goes with it, and is reset to what a restored disk holds.
// The platform labels the container it makes as this one, with its spec
// beside (LabelSpec), so what the VM's dockerd holds is matched to its record
// exactly, and a record a restored disk has outlived can be taken in again
// as it was asked for.
//
// Its spec is what it was asked for, Docker's own container spec, and never
// changes but for its networks, which connect and disconnect change, as the
// dashboard always could. Its status is what its VM's dockerd last said of it
// (Docker), in the kind's words besides: running, stopped, or completed for a
// one-off that ran to its end, which its restart policy says it is, rather
// than broken.
//
// What is decided of it, given what it was asked to be and what it is:
//
//	expected  observed                       asked
//	any       pending or missing, VM running  create, pulling its image
//	stopped   running                        stop
//	running   stopped                        start: only a container docker
//	                                         restarts itself is ever stopped
//	                                         while it is expected running
//	running   completed                      nothing: a one-off that ended
//	any       on a network it is not on      connect it
package container

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

const (
	// Name is the kind's word, and Plural what its routes are named by. Its
	// permissions are workload.containers.<verb>.
	Name   = "container"
	Plural = "containers"

	// Parent is the kind a container lives in: a Docker VM.
	Parent = blocks.Parent
)

// A container's states. Waiting, Missing, Failed and Deleted are the
// framework's own.
const (
	// Pending is a container asked for and not made yet: its VM is still
	// coming up, or its create is on its way.
	Pending kind.State = "pending"

	// Creating is a container being made: its image pulled when its VM does
	// not hold it, then made and started.
	Creating kind.State = "creating"

	Running  kind.State = "running"
	Starting kind.State = "starting"
	Stopping kind.State = "stopping"

	// Stopped is a container that is not running and is to be started again:
	// one stopped on purpose, or one docker restarts itself and has not yet.
	Stopped kind.State = "stopped"

	// Completed is a container that ran to its end and is not to be started
	// again, as its restart policy, no or on-failure, says: a one-off that
	// finished, which is not broken.
	Completed kind.State = "completed"

	Restarting kind.State = "restarting"

	// Removing is a container being removed. Its record goes once it is.
	Removing kind.State = "removing"

	// Waiting is a container whose VM is not running, so nothing of it can
	// be seen. Its reason says what its VM is doing.
	Waiting = kind.Waiting

	// Missing is a container its VM's dockerd does not have: it is made again.
	Missing = kind.Missing

	Failed  = kind.Failed
	Deleted = kind.Deleted
)

// A container's actions.
const (
	// ActionCreate makes a container that is not there, pulling its image
	// first when its VM does not hold it. It is the workload's own to ask
	// for.
	ActionCreate = "create"

	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"

	// ActionConnect and ActionDisconnect change the networks a container is
	// on, which are the one part of its spec that changes.
	ActionConnect    = "connect"
	ActionDisconnect = "disconnect"

	// ActionDelete removes a container; one that runs only by force
	// (DeletePayload).
	ActionDelete = "delete"

	// ActionState is what a container is doing, as its VM's dockerd says;
	// ActionLogs reads what it wrote (LogsPayload), and ActionStats samples
	// what it uses.
	ActionState = "state"
	ActionLogs  = "logs"
	ActionStats = "stats"
)

// The labels of a container the platform made, beside the building blocks'
// own (blocks.Labels).
const (
	// LabelSpec is the spec the container was made with, as JSON: what a
	// record that a restored disk has outlived is made again from.
	LabelSpec = "workload.container.spec"
)

// The restart policies docker restarts a container by.
const (
	RestartNo            = "no"
	RestartAlways        = "always"
	RestartUnlessStopped = "unless-stopped"
	RestartOnFailure     = "on-failure"
)

// Restarts reports whether docker starts a container again by itself once it
// stops, as its restart policy says: one that does not is done once it ends.
func Restarts(policy string) bool {
	return policy == RestartAlways || policy == RestartUnlessStopped
}

// Spec is a container as it is asked for: Docker's own container spec, and
// the Docker VM it goes into.
type Spec struct {
	// VM is the Docker VM it goes into, chosen by the rules a stack's is: as
	// it is asked, one of the person's Docker VMs, named by its uuid, or one
	// made for it with whatever else it gives; as it is kept, the uuid of the
	// one it went into.
	VM stackKind.VMChoice `json:"vm"`

	// Name is docker's name for it; empty lets docker pick one.
	Name string `json:"name,omitempty"`

	// Image is pulled first when the VM does not hold it.
	Image string `json:"image"`

	Command    []string `json:"command,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`

	// Env is the environment, each as KEY=value.
	Env        []string `json:"env,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`

	Ports  []PortBinding `json:"ports,omitempty"`
	Mounts []Mount       `json:"mounts,omitempty"`

	// Networks are the VM's docker networks it is on, beside the default
	// one, and Aliases the names its neighbours on each reach it by beside its
	// own: what it was made with, and what connect and disconnect changed.
	Networks []string            `json:"networks,omitempty"`
	Aliases  map[string][]string `json:"aliases,omitempty"`

	// RestartPolicy is no, always, unless-stopped or on-failure; empty is no.
	RestartPolicy string `json:"restart_policy,omitempty"`

	// CPUs is in cores and Memory in bytes; zero is no limit beyond the VM's.
	CPUs   float64 `json:"cpus,omitempty"`
	Memory uint64  `json:"memory,omitempty"`
}

// PortBinding publishes a container port on its VM. A host port of zero lets
// docker pick one.
type PortBinding struct {
	ContainerPort port.Port `json:"container_port"`
	HostPort      port.Port `json:"host_port"`

	// Protocol is tcp or udp.
	Protocol string `json:"protocol"`
}

// Mount is something mounted into a container: a volume of its VM, a path of
// its VM (bind), or a tmpfs.
type Mount struct {
	Type     string `json:"type"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// Docker is the container spec as docker is asked to make it, labelled as the
// platform's, as the resource uuid names.
func (s Spec) Docker(uuid string, labels map[string]string) docker.ContainerSpec {
	spec := docker.ContainerSpec{
		Name:          s.Name,
		Image:         s.Image,
		Command:       clone(s.Command),
		Entrypoint:    clone(s.Entrypoint),
		Env:           clone(s.Env),
		WorkingDir:    s.WorkingDir,
		Networks:      clone(s.Networks),
		Aliases:       aliases(s.Aliases),
		RestartPolicy: s.RestartPolicy,
		CPUs:          s.CPUs,
		Memory:        s.Memory,
		Labels:        blocks.Labels(Name, uuid),
	}

	for key, value := range labels {
		spec.Labels[key] = value
	}

	for _, p := range s.Ports {
		spec.Ports = append(spec.Ports, docker.PortBinding{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: p.Protocol})
	}

	for _, m := range s.Mounts {
		spec.Mounts = append(spec.Mounts, docker.Mount{Type: m.Type, Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly})
	}

	return spec
}

// SpecOf is a container spec as a container's spec has it.
func SpecOf(s docker.ContainerSpec) Spec {
	spec := Spec{
		Name:          s.Name,
		Image:         s.Image,
		Command:       clone(s.Command),
		Entrypoint:    clone(s.Entrypoint),
		Env:           clone(s.Env),
		WorkingDir:    s.WorkingDir,
		Networks:      clone(s.Networks),
		Aliases:       aliases(s.Aliases),
		RestartPolicy: s.RestartPolicy,
		CPUs:          s.CPUs,
		Memory:        s.Memory,
	}

	for _, p := range s.Ports {
		spec.Ports = append(spec.Ports, PortBinding{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: p.Protocol})
	}

	for _, m := range s.Mounts {
		spec.Mounts = append(spec.Mounts, Mount{Type: m.Type, Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly})
	}

	return spec
}

// SpecLabel is a container's spec as the label it is made with carries it
// (LabelSpec): all of it but the VM it goes into, which it is in.
func SpecLabel(spec Spec) (string, error) {
	spec.VM = stackKind.VMChoice{}

	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

// SpecFromLabels is the spec a container was made with, read off its labels,
// and whether they say it.
func SpecFromLabels(labels map[string]string) (Spec, bool) {
	written, labelled := labels[LabelSpec]
	if !labelled {
		return Spec{}, false
	}

	var spec Spec
	if err := json.Unmarshal([]byte(written), &spec); err != nil {
		return Spec{}, false
	}

	return spec, true
}

// PolicyOf is the restart policy a container was made with, read off its
// labels, and whether they say it: no when it was made with none.
func PolicyOf(labels map[string]string) (string, bool) {
	spec, labelled := SpecFromLabels(labels)
	if !labelled {
		return "", false
	}

	if len(spec.RestartPolicy) == 0 {
		return RestartNo, true
	}

	return spec.RestartPolicy, true
}

// Status is what a container is doing: its state, and what its VM's dockerd
// last said of it beside it, in one object. Docker has a state of its own, so
// each is read where it is from: s.Status.State is the container's, in the
// kind's words, and s.Docker.State docker's.
type Status struct {
	kind.Status

	// Docker is the container as its VM's dockerd last had it, its fields
	// beside the state's, and nothing until it is made.
	*Docker

	// Failure is what the last command on it failed with, in the codes every
	// side knows, so that whoever waited for the command says it as its node
	// did; an empty one is a command that did not fail since.
	Failure *noderequest.Error `json:"failure,omitempty"`
}

// Docker is a container as its VM's dockerd has it.
//
// Each of its fields is written whenever it is there, empty or not: a status
// taken onto a record keeps a field the report leaves out (resource.Merge),
// so what docker says of it takes the place of all it said before.
type Docker struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`

	// State is docker's: created, running, paused, restarting, removing,
	// exited or dead. Status is the sentence docker puts it in. Beside a
	// status's own state, they are docker_state and docker_status.
	State  string `json:"docker_state"`
	Status string `json:"docker_status"`

	Command  string            `json:"command"`
	Ports    []PortBinding     `json:"ports"`
	Networks []string          `json:"networks"`
	Mounts   []Mount           `json:"mounts"`
	Labels   map[string]string `json:"labels"`

	RestartPolicy string    `json:"restart_policy"`
	CreatedAt     time.Time `json:"created_at"`
}

// DockerOf is a container as docker has it, as a status has it.
func DockerOf(c docker.Container) *Docker {
	d := &Docker{
		ID:            c.ID,
		Name:          c.Name,
		Image:         c.Image,
		State:         c.State,
		Status:        c.Status,
		Command:       c.Command,
		Ports:         []PortBinding{},
		Networks:      clone(c.Networks),
		Mounts:        []Mount{},
		Labels:        c.Labels,
		RestartPolicy: c.RestartPolicy,
		CreatedAt:     c.CreatedAt,
	}

	if d.Networks == nil {
		d.Networks = []string{}
	}

	for _, p := range c.Ports {
		d.Ports = append(d.Ports, PortBinding{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: p.Protocol})
	}

	for _, m := range c.Mounts {
		d.Mounts = append(d.Mounts, Mount{Type: m.Type, Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly})
	}

	return d
}

// Container is it as docker has it.
func (d *Docker) Container() docker.Container {
	if d == nil {
		return docker.Container{}
	}

	c := docker.Container{
		ID:            d.ID,
		Name:          d.Name,
		Image:         d.Image,
		State:         d.State,
		Status:        d.Status,
		Command:       d.Command,
		Networks:      clone(d.Networks),
		Labels:        d.Labels,
		Stack:         d.Labels[docker.LabelComposeProject],
		Service:       d.Labels[docker.LabelComposeService],
		RestartPolicy: d.RestartPolicy,
		CreatedAt:     d.CreatedAt,
	}

	for _, p := range d.Ports {
		c.Ports = append(c.Ports, docker.PortBinding{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: p.Protocol})
	}

	for _, m := range d.Mounts {
		c.Mounts = append(c.Mounts, docker.Mount{Type: m.Type, Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly})
	}

	return c
}

// StateOf is what a container docker has in a state is doing, in the kind's
// words, given its restart policy: a container that is up is running, one
// that ran and exited is completed when docker does not start it again by
// itself, and anything else that is not up is stopped.
func StateOf(dockerState string, restartPolicy string) kind.State {
	switch dockerState {
	case "running", "restarting", "paused":
		return Running
	case "exited":
		if !Restarts(restartPolicy) {
			return Completed
		}
	}

	return Stopped
}

// ConnectPayload is a network a container is put on, and the names its
// neighbours there reach it by beside its own.
type ConnectPayload struct {
	Network string   `json:"network"`
	Aliases []string `json:"aliases,omitempty"`
}

var _ domain.Validatable = &ConnectPayload{}

func (p *ConnectPayload) Validate() domain.ValidationErrors {
	if len(strings.TrimSpace(p.Network)) == 0 {
		return domain.ValidationErrors{"network": "required_field"}
	}

	return nil
}

// DisconnectPayload is a network a container is taken off.
type DisconnectPayload struct {
	Network string `json:"network"`

	// Force takes it off a network docker would otherwise keep it on.
	Force bool `json:"force,omitempty"`
}

var _ domain.Validatable = &DisconnectPayload{}

func (p *DisconnectPayload) Validate() domain.ValidationErrors {
	if len(strings.TrimSpace(p.Network)) == 0 {
		return domain.ValidationErrors{"network": "required_field"}
	}

	return nil
}

// DeletePayload is how a container is removed.
type DeletePayload struct {
	// Force removes one that is still running; without it, one that runs is
	// refused.
	Force bool `json:"force,omitempty"`
}

var _ domain.Validatable = &DeletePayload{}

// Validate says nothing is wrong: either way is a delete.
func (p *DeletePayload) Validate() domain.ValidationErrors {
	return nil
}

// LogsPayload narrows what is read of a container's log.
type LogsPayload struct {
	// Since leaves out what was written before it; nothing is from the
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

// Logs are the lines of a container's log a query read, oldest first.
type Logs struct {
	Lines     []LogLine `json:"lines"`
	Truncated bool      `json:"truncated,omitempty"`
}

// LogLine is one line a container wrote, and whether it went to stdout or
// to stderr.
type LogLine struct {
	At     time.Time `json:"at"`
	Stream string    `json:"stream"`
	Line   string    `json:"line"`
}

// Stats is one sample of what a container uses: Docker's, as it travels.
type Stats struct {
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsed  uint64    `json:"memory_used"`
	MemoryLimit uint64    `json:"memory_limit"`
	NetworkRx   uint64    `json:"network_rx"`
	NetworkTx   uint64    `json:"network_tx"`
	BlockRead   uint64    `json:"block_read"`
	BlockWrite  uint64    `json:"block_write"`
	PIDs        uint64    `json:"pids"`
	SampledAt   time.Time `json:"sampled_at"`
}

// StatsOf is a sample docker took, as it travels.
func StatsOf(s docker.Stats) Stats {
	return Stats(s)
}

// Docker is the sample as docker took it.
func (s Stats) Docker() docker.Stats {
	return docker.Stats(s)
}

// Container is a container, as its strategies are handed one.
type Container = kind.Resource[Spec, Status]

// VMOf is the uuid of the Docker VM a container lives in: its parent.
func VMOf(c Container) string {
	if parent, ok := c.Metadata.Owner(Parent); ok {
		return parent.UUID
	}

	return c.Spec.VM.UUID
}

// Descriptor is the container kind, a new one every time.
func Descriptor() kind.Descriptor {
	command := func(name string, allowedIn []kind.State, desires kind.State, payload kind.Codec) kind.Action {
		return kind.Action{Name: name, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: allowedIn, Desires: desires, Permission: "manage", Payload: payload}
	}

	waiting := func(a kind.Action) kind.Action {
		a.Waits = true

		return a
	}

	// what makes a container, which a start or a restart does when its VM
	// has none of it, pulls its image first when its VM does not hold it.
	pulling := func(a kind.Action) kind.Action {
		a.Timeout = kind.TimeoutPull

		return a
	}

	there := []kind.State{Running, Stopped, Completed}

	return kind.Descriptor{
		Name:          Name,
		Plural:        Plural,
		PermissionsOf: blocks.PermissionsOf,
		StateBy:       kind.OnNode,
		Parent:        Parent,
		OnParent:      blocks.OnParent(),
		Machine:       Machine(),
		Actions: []kind.Action{
			{Name: ActionCreate, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Pending, Missing}, Internal: true, Timeout: kind.TimeoutPull, Payload: kind.NoPayload},
			pulling(waiting(command(ActionStart, []kind.State{Stopped, Completed, Missing, Failed}, Running, kind.NoPayload))),
			waiting(command(ActionStop, []kind.State{Running, Failed}, Stopped, kind.NoPayload)),
			pulling(waiting(command(ActionRestart, []kind.State{Running, Stopped, Completed, Missing, Failed}, Running, kind.NoPayload))),
			command(ActionConnect, there, "", kind.Payload[ConnectPayload]()),
			command(ActionDisconnect, there, "", kind.Payload[DisconnectPayload]()),
			{Name: ActionDelete, Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: Deleted, Permission: "delete", Payload: kind.Payload[DeletePayload]()},
			{Name: ActionState, Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
			{Name: ActionLogs, Runs: kind.OnNode, Mode: kind.ModeQuery, AllowedIn: []kind.State{Running, Stopped, Completed, Starting, Stopping, Restarting, Failed}, Permission: "logs", Payload: kind.Payload[LogsPayload]()},
			{Name: ActionStats, Runs: kind.OnNode, Mode: kind.ModeQuery, AllowedIn: []kind.State{Running}, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// Machine is a container's states and the moves between them.
//
// A command is waited on in flight until its own answer, or its node, says
// where it got: creating, starting and restarting end in running, stopped or
// completed, stopping in stopped or completed, and removing only in the
// container being gone. A start and a restart are over only once they are
// answered: a container runs before a restart as it does after it, and a
// look its node took between the start being asked and carried out says it
// is stopped, as it was, which says nothing of what the start came to.
//
// At rest, a container is what its node says: running, stopped or completed,
// missing when its VM's dockerd has none of it, and waiting while its VM is
// not running.
func Machine() kind.Machine {
	transitions := []kind.Transition{
		{From: kind.Any, On: kind.OnAction(ActionDelete), To: Removing},
		{From: Removing, On: kind.OnObserved(Missing), To: Deleted},
		{From: kind.Any, On: kind.OnObserved(Failed), To: Failed},
	}

	asked := func(action string, to kind.State, from ...kind.State) {
		for _, state := range from {
			transitions = append(transitions, kind.Transition{From: state, On: kind.OnAction(action), To: to})
		}
	}

	asked(ActionCreate, Creating, Pending, Missing)
	asked(ActionStart, Starting, Stopped, Completed, Missing, Failed)
	asked(ActionStop, Stopping, Running, Failed)
	asked(ActionRestart, Restarting, Running, Stopped, Completed, Missing, Failed)

	arrives := func(from kind.State, at ...kind.State) {
		for _, state := range at {
			transitions = append(transitions, kind.Transition{From: from, On: kind.OnObserved(state), To: state})
		}
	}

	arrives(Creating, Running, Stopped, Completed)
	arrives(Starting, Running, Stopped, Completed)
	arrives(Restarting, Running, Stopped, Completed)
	arrives(Stopping, Stopped, Completed)

	observed := func(at kind.State, from ...kind.State) {
		for _, state := range from {
			transitions = append(transitions, kind.Transition{From: state, On: kind.OnObserved(at), To: at})
		}
	}

	// one its VM has none of is made again; one in a VM that is not running
	// waits on it.
	observed(Missing, Pending, Running, Stopped, Completed, Waiting, Failed)
	observed(Waiting, Pending, Running, Stopped, Completed, Missing, Failed)

	return kind.Machine{
		Initial:     Pending,
		States:      []kind.State{Pending, Creating, Running, Starting, Stopping, Stopped, Completed, Restarting, Removing, Waiting, Missing, Failed, Deleted},
		Transitions: transitions,
		Terminal:    []kind.State{Stopped, Completed, Failed, Deleted},
		InFlight:    []kind.State{Creating, Starting, Stopping, Restarting, Removing},
		Answered:    []kind.State{Starting, Restarting},
	}
}

func clone(values []string) []string {
	if values == nil {
		return nil
	}

	return append([]string(nil), values...)
}

func aliases(of map[string][]string) map[string][]string {
	if of == nil {
		return nil
	}

	copied := make(map[string][]string, len(of))
	for network, names := range of {
		copied[network] = clone(names)
	}

	return copied
}
