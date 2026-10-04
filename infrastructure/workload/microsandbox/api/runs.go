package api

import "time"

// NetworkPolicy is what a run's guest may reach, and what may reach it.
//
// There is no "none". Microsandbox cannot make a sandbox without a network
// interface, only one whose traffic is denied, which is not what none
// promises, so the orchestrator refuses such a task before it gets here.
type NetworkPolicy string

const (
	// NetworkIsolated is no egress at all, DNS included, and the guest is
	// reached only on its published ports. Unlike docker's shared isolated
	// network, isolated runs do not reach each other by name: microsandbox
	// has no network between sandboxes.
	NetworkIsolated NetworkPolicy = "isolated"

	// NetworkPublic is the public internet, and never private ranges, the
	// host or a cloud's metadata service.
	NetworkPublic NetworkPolicy = "public"
)

// State is where a run is in its life.
//
// It is finer than docker's statuses because the service does more between
// them: a start pulls an image and boots a VM before anything runs, and a stop
// waits for the main process before the VM goes. A client reads created and
// starting as a container that was created, running and stopping as one that
// runs, restarting as one restarting, and exited as one that exited.
type State string

const (
	StateCreated    State = "created"    // recorded, and never booted
	StateStarting   State = "starting"   // pulling, booting, or spawning the main process
	StateRunning    State = "running"    // the main process runs
	StateStopping   State = "stopping"   // asked to stop, and still running
	StateRestarting State = "restarting" // waiting out the restart policy's backoff
	StateExited     State = "exited"     // the main process ended, and the VM is stopped
)

// Task is what a run is running, kept with the run and handed back exactly as
// it was given. It is what lets a node say what it holds without asking
// anything that keeps records, as docker keeps the same facts in a
// container's labels. The service reads nothing in it but UUID and Slug,
// which a listing can be narrowed by.
type Task struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`

	// Slug is the name the task's ports are served under.
	Slug string `json:"slug"`

	// Kind is "job" or "service", as the workload names them.
	Kind string `json:"kind"`

	// Owner and Stack are the UUIDs of whoever owns the task and of the
	// stack it belongs to. Each is empty when there is none.
	Owner string `json:"owner,omitempty"`
	Stack string `json:"stack,omitempty"`

	// Attempt is which run of the task this is: a retry is a new run of the
	// same task, and the one it replaces may still be there.
	Attempt int `json:"attempt"`

	// Interactive is a task meant to be watched and reached while it runs,
	// rather than waited on for what it prints.
	Interactive bool `json:"interactive,omitempty"`

	// TTLSeconds is how long the task may run once it is up. Zero is no
	// limit.
	TTLSeconds int64 `json:"ttl_seconds,omitempty"`
}

// RunSpec is what a run was asked to be.
//
// Entrypoint and Command follow docker's rules rather than microsandbox's: an
// empty one takes the image's, and an Entrypoint given without a Command drops
// the image's CMD, where microsandbox would keep it. The service works out the
// program and its arguments itself, so a task runs the same thing on either
// runtime.
type RunSpec struct {
	// Node is the orchestrator the run belongs to. Every listing is scoped
	// to one, as a container's label scopes it on the shared docker daemon.
	Node string `json:"node"`

	// Name is unique on its node among the runs that exist, so asking to
	// create a run twice is 409 name_in_use rather than a second run.
	Name string `json:"name"`

	Image      string   `json:"image"`
	Entrypoint []string `json:"entrypoint,omitempty"`
	Command    []string `json:"command,omitempty"`

	// Environment is KEY=VALUE entries laid over the image's. None may hold
	// a tab, which microsandbox's guest refuses, or a NUL, which no
	// environment can carry.
	Environment []string `json:"environment,omitempty"`
	WorkingDir  string   `json:"working_dir,omitempty"`

	// CPU is in cores. Microsandbox gives whole vCPUs, so a run gets
	// ceil(CPU) of them and at least one: a fractional limit is not
	// enforced, and a task given a quarter of a core may use all of one.
	CPU float64 `json:"cpu"`

	// Memory is in bytes, and raised to the service's floor, the least a
	// guest boots in.
	Memory uint64 `json:"memory"`

	// Disk is in bytes: the size of the run's writable root. Zero leaves it
	// at microsandbox's own 4 GiB.
	Disk uint64 `json:"disk"`

	Network NetworkPolicy `json:"network"`

	// Ports are the guest's TCP ports to publish. The service picks a host
	// port for each at the run's first boot and keeps it for as long as the
	// run exists, because microsandbox fixes a sandbox's ports when it is
	// created.
	Ports []uint16 `json:"ports,omitempty"`

	// RestartPolicy is docker's: "no", "always", "on-failure",
	// "on-failure:N" or "unless-stopped", and empty is "no". The service
	// applies it, since microsandbox has none of its own.
	RestartPolicy string `json:"restart_policy,omitempty"`

	Task Task `json:"task"`
}

// Endpoint is one of a run's published ports. The orchestrator reaches the
// guest's Port at HostPort on the service's address, and reports that for the
// task's port as it reports a container's port bindings.
type Endpoint struct {
	Port     uint16 `json:"port"`      // the guest's
	HostPort uint16 `json:"host_port"` // published on the service's bind address
}

// Run is one run of a task in a microVM: what it was asked to be, and what it
// has become. It is the service's counterpart of a container, and what a
// client turns into the workload's own execution of a task.
type Run struct {
	// ID is the service's name for the run, and opaque to a client.
	ID string `json:"id"`

	RunSpec

	State State `json:"state"`

	// ExitCode is how the main process ended, numbered as docker numbers it
	// so the workload reads it as it reads a container's: N for a process
	// that exited with N, 128 plus the signal's number for one the stop
	// signal ended, and 137 for one that was killed or whose VM was lost.
	// A program that never started is 127 when it was not found, 126 when
	// it could not be executed, and 1 for anything else.
	ExitCode int `json:"exit_code"`

	// Error says why a run ended when its exit code cannot: "killed" for a
	// signal nobody sent, such as the guest running out of memory,
	// "vm_lost" for a VM that died, and "service_restarted" for a service
	// that went away under it.
	Error string `json:"error,omitempty"`

	// RestartCount counts the restart policy's restarts only, as docker's
	// does: a restart someone asked for is not one of them.
	RestartCount uint `json:"restart_count"`

	// Endpoints are the run's published ports, and there only while it
	// runs.
	Endpoints []Endpoint `json:"endpoints,omitempty"`

	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
}

// RunList is a node's runs, as narrowed as the listing asked. The service
// sends an empty list rather than null, and a client takes either as nothing.
type RunList struct {
	Runs []Run `json:"runs"`
}

// StopRequest is the optional body of a stop or a restart: how long the main
// process is given between the stop signal and SIGKILL. Zero, or no body at
// all, is 10 seconds, as it is docker's.
type StopRequest struct {
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}
