// Package task is the task kind: a program run once, or kept running, in an
// ephemeral VM of its own on one node, declared once for every service that
// runs it (domain/workload/kind). The public code runner runs every snippet
// as a task of the guest's (task.GuestOwnerUUID).
//
// A task is a job or a service (task.Kind). A job runs once, and what it was
// is what it printed: once it has ended, completed, stopped or failed, it is
// deleted, VM and all. A service is meant to keep running, and one that ends
// unasked is asked for again, as many times as its spec says.
//
// Its spec is today's task's: the image its VM boots, the command it runs,
// its environment, the ports it serves, its network policy, its limits,
// whether it is watched while it runs, its ttl and how many times it is worth
// asking for again. Its ttl is how long a job may run, counted from when its
// run started rather than from when it was asked for: a job past its ttl is
// stopped, and one that never starts is never stopped for it.
//
// Its status is what its node last said of it: its state, and its run, which
// is the VM its node holds of it, read off the VM itself (Run). A job's
// output rides its run, so whoever follows a task from its node's
// heartbeats, the code runner, answers from it alone.
//
// Its states are today's: created, scheduled, running, restarting,
// stopping, stopped, completed and failed, deleting while its node takes it
// away, and deleted once it is gone. A job that exits with 0, or that was
// stopped, completed; one that exits with anything else it chose failed.
package task

import (
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

const (
	// Name is the kind's word, and Plural what its routes are named by. Its
	// permissions are the VMs' (PermissionsOf): the code runner's tasks are
	// shown, stopped and deleted among anybody's VMs.
	Name   = "task"
	Plural = "tasks"
)

// A task's states. Failed and Deleted are the framework's own.
const (
	// Created is a task admitted and not yet asked of its node.
	Created kind.State = "created"

	// Scheduled is a task its node has been asked to run, and has not run
	// yet: its VM is being made, its image pulled.
	Scheduled kind.State = "scheduled"

	Running kind.State = "running"

	// Restarting is a task whose run is being restarted in place, as its
	// node says.
	Restarting kind.State = "restarting"

	Stopping kind.State = "stopping"

	// Stopped is a service that ended, and a job its node holds nothing of
	// once it was stopped.
	Stopped kind.State = "stopped"

	// Completed is a job that ran to its end: its program exited with 0, or
	// was stopped.
	Completed kind.State = "completed"

	// Deleting is a task its node has been asked to take away. Its record
	// goes once its node says it has.
	Deleting kind.State = "deleting"

	// Failed is a task that could not be run, whose program exited with a
	// code of its own choosing other than 0, or whose node fell silent. Its
	// reason says why.
	Failed = kind.Failed

	Deleted = kind.Deleted
)

// A task's actions.
const (
	// ActionCreate runs a task on its node: a VM made for it and booted,
	// which runs its command. It is the workload's own to ask for, as a task
	// is admitted and whenever a failed one is worth another attempt.
	ActionCreate = "create"

	// ActionStop and ActionKill stop a task's run. A VM has no grace period to
	// cut short, so the two end it alike; a job past its ttl is killed.
	ActionStop = "stop"
	ActionKill = "kill"

	// ActionDelete takes a task away from its node, VM and all.
	ActionDelete = "delete"

	// ActionState is what a task is doing, as its node says; ActionLogs reads
	// what its run has written (LogsPayload).
	ActionState = "state"
	ActionLogs  = "logs"

	// ActionAttach opens a terminal in a running task: anybody's, signed in
	// or not, for a task of the guest's, and its owner's alone otherwise.
	ActionAttach = "attach"
)

// Limits are what a task's VM is held to: cores, of which its VM is given
// whole vCPUs, rounded up, and bytes of memory and disk, as they are
// everywhere in the workload.
type Limits struct {
	CPU    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	Disk   uint64  `json:"disk"`
}

// ResourceLimits are the limits as a run is given them.
func (l Limits) ResourceLimits() task.ResourceLimits {
	return task.ResourceLimits{Cpu: l.CPU, Memory: l.Memory, Disk: l.Disk}
}

// Spec is what a task is asked for as. It never changes.
type Spec struct {
	// Kind is a job, which runs once, or a service, which keeps running. A
	// spec that names none is a job.
	Kind task.Kind `json:"kind"`

	Image       string   `json:"image"`
	Entrypoint  []string `json:"entrypoint,omitempty"`
	Command     []string `json:"command,omitempty"`
	Environment []string `json:"environment,omitempty"`

	// Ports are the ports its program serves, which the ingress serves under
	// its slug while it runs.
	Ports []port.Port `json:"ports,omitempty"`

	// NetworkPolicy is how much of the network it reaches: none, isolated,
	// which serves its ports and calls nothing, or public. None names the
	// default, isolated.
	NetworkPolicy network.Policy `json:"network_policy,omitempty"`

	// Interactive says it is watched and reached while it runs, rather than
	// waited on for what it prints.
	Interactive bool `json:"interactive,omitempty"`

	// TTL is how long a job may run, in nanoseconds as every duration
	// between the workload's services is, counted from when its run started.
	// Zero is no limit; a service has none.
	TTL time.Duration `json:"ttl,omitempty"`

	Limits Limits `json:"limits"`

	// MaxRetries is how many times it is asked for again after it fails,
	// task.RetryForever for as long as it keeps failing. Nothing is what its
	// kind is usually worth (task.DefaultMaxRetries), which is what a task is
	// admitted with.
	MaxRetries *int `json:"max_retries,omitempty"`
}

// Retries is how many times a task is worth asking for again after it
// fails.
func (s Spec) Retries() int {
	if s.MaxRetries == nil {
		return task.DefaultMaxRetries(s.TaskKind())
	}

	return *s.MaxRetries
}

// TaskKind is the kind of task it is: a job unless it says otherwise.
func (s Spec) TaskKind() task.Kind {
	if s.Kind.IsValid() {
		return s.Kind
	}

	return task.DefaultKind
}

// Policy is its network policy, the default one when it names none.
func (s Spec) Policy() network.Policy {
	if len(s.NetworkPolicy) == 0 {
		return network.DefaultPolicy
	}

	return s.NetworkPolicy
}

// Status is what a task is doing: its state, and its run, as its node last
// said.
type Status struct {
	kind.Status

	// Retries is how many times it was asked for again after it failed,
	// which the control plane counts as it asks. No node says anything of
	// it.
	Retries int `json:"retries,omitempty"`

	// Run is the run of it its node holds: nothing for a task no node has run
	// yet.
	Run *Run `json:"run,omitempty"`
}

// Run is one run of a task, as the node holding it says.
//
// A run is made from the task as it was recorded, and carries what it was
// made as on the VM it runs in, which its node reads back off the VM: its
// name, its slug, its kind, whether it is watched, and which attempt at the
// task it is. That is what lets whoever follows a task from its node's
// heartbeats answer for it without asking anything that keeps records: the
// code runner answers the request a snippet's task is named after.
type Run struct {
	// ID is the run's on its node.
	ID string `json:"id,omitempty"`

	// Attempt is which attempt at the task the run is, counted from 0.
	Attempt int `json:"attempt,omitempty"`

	Name        string    `json:"name,omitempty"`
	Slug        string    `json:"slug,omitempty"`
	Kind        task.Kind `json:"kind,omitempty"`
	Interactive bool      `json:"interactive,omitempty"`

	// StartedAt is when its program started, as its node says, and Deadline
	// when it will be stopped for having run long enough: its ttl after that.
	// A run that has not started, or may run as long as it likes, has none.
	StartedAt time.Time `json:"started_at,omitzero"`
	Deadline  time.Time `json:"deadline,omitzero"`

	// ExitCode is what its program exited with, once it has.
	ExitCode int `json:"exit_code,omitempty"`

	// Output is what a job's program has written so far, its last
	// MaxOutput bytes at most: what a snippet printed. A service's is shipped
	// a line at a time instead, and kept by the control plane.
	Output string `json:"output,omitempty"`

	// Endpoints are the ports of it that came up while it runs, and where
	// its node reaches each.
	Endpoints []Endpoint `json:"endpoints,omitempty"`
}

// Job reports whether a run is of a job.
func (r *Run) Job() bool {
	return r != nil && r.Kind != task.KindService
}

// Endpoint is a port of a running task and the address its node reaches it
// at.
type Endpoint struct {
	Port    port.Port `json:"port"`
	Address string    `json:"address"`
}

// MaxOutput is the most of a job's output its run carries, in bytes, counted
// from the end. It rides every heartbeat of its node, beside every other
// kind's state, which has to fit in one message.
const MaxOutput = 256 << 10

// LogsPayload narrows what is read of a task's log.
type LogsPayload struct {
	// Tail keeps only the last lines; nothing is as many as an answer
	// carries.
	Tail uint `json:"tail,omitempty"`
}

var _ domain.Validatable = &LogsPayload{}

// Validate says nothing is wrong: any part of a log can be read.
func (p *LogsPayload) Validate() domain.ValidationErrors {
	return nil
}

// Logs are the lines a task's run has written, oldest first, and whether
// there were more of them than an answer carries.
type Logs struct {
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated,omitempty"`
}

// Task is a task, as its strategies are handed one.
type Task = kind.Resource[Spec, Status]

// Ended reports whether a task in state s has come to its end: its run
// completed, stopped or failed.
func Ended(s kind.State) bool {
	return s == Completed || s == Stopped || s == Failed
}

// StateOf is what a run's status says a task is doing, given its kind and
// what its program exited with.
//
// The same status says different things of the two kinds. A job that exits
// has finished, which is the whole point of running it, unless its program
// returned a failure: a job that ends badly has not completed. One killed by
// a signal, 128 and above, was cut short rather than failed, as a stopped job
// is. A service that exits has stopped: it was meant to keep going, and
// whether it was supposed to stop is for whoever knows what was asked of it.
func StateOf(status task.Status, k task.Kind, exitCode int) kind.State {
	switch status {
	case task.StatusCreated:
		return Scheduled
	case task.StatusRunning:
		return Running
	case task.StatusRestarting:
		return Restarting
	case task.StatusPaused:
		return Stopped
	case task.StatusExited, task.StatusRemoving:
		if k == task.KindService {
			return Stopped
		}

		if exitCode != 0 && exitCode < 128 {
			return Failed
		}

		return Completed
	}

	return Failed
}

// Descriptor is the task kind, a new one every time.
func Descriptor() kind.Descriptor {
	command := func(name string, allowedIn []kind.State, desires kind.State, permission string) kind.Action {
		return kind.Action{Name: name, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: allowedIn, Desires: desires, Permission: permission, Payload: kind.NoPayload}
	}

	stoppable := []kind.State{Scheduled, Running, Restarting}

	return kind.Descriptor{
		Name:          Name,
		Plural:        Plural,
		StateBy:       kind.OnNode,
		Endpoints:     true,
		PermissionsOf: vmKind.Plural,
		Machine:       Machine(),
		Actions: []kind.Action{
			{Name: ActionCreate, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Created, Failed, Stopped}, Desires: Running, Internal: true, Payload: kind.NoPayload},
			command(ActionStop, stoppable, Stopped, "manage"),
			command(ActionKill, slices.Clone(stoppable), Stopped, "manage"),
			command(ActionDelete, nil, Deleted, "delete"),
			{Name: ActionState, Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
			{Name: ActionLogs, Runs: kind.OnNode, Mode: kind.ModeQuery, AllowedIn: []kind.State{Running, Restarting, Stopping, Stopped, Completed, Failed}, Permission: "logs", Payload: kind.Payload[LogsPayload]()},
			{Name: ActionAttach, Runs: kind.OnNode, Mode: kind.ModeStream, AllowedIn: []kind.State{Running}, Permission: "attach", Public: true, Payload: kind.NoPayload},
		},
	}
}

// Machine is a task's states and the moves between them.
//
// A command is waited on in flight until its own answer, or its node, says
// where it got: scheduled ends in the run running, or already ended,
// stopping in it having ended, and deleting in the task being gone. At rest,
// a task is what its node says. A running task its node no longer holds has
// failed; a stopped one its node holds nothing of has stopped.
func Machine() kind.Machine {
	transitions := []kind.Transition{
		{From: kind.Any, On: kind.OnAction(ActionDelete), To: Deleting},
		{From: Deleting, On: kind.OnObserved(kind.Missing), To: Deleted},
		{From: kind.Any, On: kind.OnObserved(Failed), To: Failed},
		{From: Running, On: kind.OnObserved(kind.Missing), To: Failed},
		{From: Restarting, On: kind.OnObserved(kind.Missing), To: Failed},
		{From: Stopping, On: kind.OnObserved(kind.Missing), To: Stopped},
	}

	asked := func(action string, to kind.State, from ...kind.State) {
		for _, state := range from {
			transitions = append(transitions, kind.Transition{From: state, On: kind.OnAction(action), To: to})
		}
	}

	asked(ActionCreate, Scheduled, Created, Failed, Stopped)
	asked(ActionStop, Stopping, Scheduled, Running, Restarting)
	asked(ActionKill, Stopping, Scheduled, Running, Restarting)

	arrives := func(from kind.State, at ...kind.State) {
		for _, state := range at {
			transitions = append(transitions, kind.Transition{From: from, On: kind.OnObserved(state), To: state})
		}
	}

	arrives(Scheduled, Running, Restarting, Stopped, Completed)
	arrives(Stopping, Stopped, Completed)

	return kind.Machine{
		Initial:     Created,
		States:      []kind.State{Created, Scheduled, Running, Restarting, Stopping, Stopped, Completed, Failed, Deleting, Deleted},
		Transitions: transitions,
		Terminal:    []kind.State{Stopped, Completed, Failed, Deleted},
		InFlight:    []kind.State{Scheduled, Stopping, Deleting},
	}
}
