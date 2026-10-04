package client

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// anyAddress is the address a run's port is reported bound on, as docker
// reports a container's port published on all of its host's addresses. The
// service binds one address of its own, but nothing reads this one: a port is
// reached at the host of the service's URL, wherever it was bound.
const anyAddress = "0.0.0.0"

// runSpec is what the service is asked to make of one of the workload's
// executions, or why microsandbox cannot run it at all.
//
// The run belongs to the node the execution names, as a container is labelled
// with it, or to the node this client works for when it names none.
func runSpec(execution *task.Execution, node string) (api.RunSpec, error) {
	if err := refuse(execution); err != nil {
		return api.RunSpec{}, err
	}

	ports, err := publishedPorts(execution)
	if err != nil {
		return api.RunSpec{}, err
	}

	if len(execution.NodeName) > 0 {
		node = execution.NodeName
	}

	return api.RunSpec{
		Node:          node,
		Name:          execution.Name,
		Image:         execution.Image,
		Entrypoint:    execution.Entrypoint,
		Command:       execution.Command,
		Environment:   execution.Environment,
		WorkingDir:    execution.WorkingDirectory,
		CPU:           execution.ResourceLimits.Cpu,
		Memory:        execution.ResourceLimits.Memory,
		Disk:          execution.ResourceLimits.Disk,
		Network:       policyOf(execution.Networks),
		Ports:         ports,
		RestartPolicy: execution.RestartPolicy,

		// what the run is running, which the service keeps with it and hands
		// back as it was given, as docker keeps it in a container's labels.
		Task: api.Task{
			UUID:        execution.TaskUUID,
			Name:        execution.TaskName,
			Slug:        execution.Slug,
			Kind:        string(kindOf(execution.Kind)),
			Owner:       execution.OwnerUUID,
			Stack:       execution.StackUUID,
			Attempt:     execution.Attempt,
			Interactive: execution.Interactive,
			TTLSeconds:  ttlSeconds(execution.TTL),
		},
	}, nil
}

// policyOf is how much of the network an execution's attachments let it reach.
// Only the default bridge routes out, so an execution on it is public, and one
// on anything else is isolated.
func policyOf(attachments []network.Attachment) api.NetworkPolicy {
	for _, attachment := range attachments {
		if attachment.Name == network.PublicNetworkName || attachment.Gateway {
			return api.NetworkPublic
		}
	}

	return api.NetworkIsolated
}

// publishedPorts are the guest's ports to publish: every port an execution
// exposes or asks to have bound.
//
// The service picks the host port of each, as docker does for a binding that
// names none, which is the only kind the workload asks for. A host port asked
// for anyway is not kept.
func publishedPorts(execution *task.Execution) ([]uint16, error) {
	wanted := make(map[port.Port]struct{}, len(execution.ExposedPorts)+len(execution.PortBindings))

	for exposed := range execution.ExposedPorts {
		wanted[exposed] = struct{}{}
	}

	for bound := range execution.PortBindings {
		wanted[bound] = struct{}{}
	}

	if len(wanted) == 0 {
		return nil, nil
	}

	ports := make([]uint16, 0, len(wanted))
	for p := range wanted {
		if p == 0 || p > math.MaxUint16 {
			return nil, fmt.Errorf("%d is not a TCP port, so it cannot be published", p)
		}

		ports = append(ports, uint16(p))
	}

	slices.Sort(ports)

	return ports, nil
}

// ttlSeconds is how long a task may run once it is up, in the whole seconds the
// service keeps it in. It is rounded up, so that a limit, however short, is
// never read back as none.
func ttlSeconds(ttl time.Duration) int64 {
	if ttl <= 0 {
		return 0
	}

	return int64((ttl + time.Second - 1) / time.Second)
}

// kindOf is a task's kind, or the default for one that names none, as docker's
// labels are read back.
func kindOf(kind task.Kind) task.Kind {
	if kind.IsValid() {
		return kind
	}

	return task.DefaultKind
}

// statuses are the service's states as the workload reads a container's.
//
// A run that is starting has run nothing yet, since it is pulling its image or
// booting its VM, which is what a container that was created and never started
// looks like. A run that is stopping still runs, as a container docker is
// stopping does until it has stopped. A state this client does not know is no
// status at all, as an unknown one of docker's is.
var statuses = map[api.State]task.Status{
	api.StateCreated:    task.StatusCreated,
	api.StateStarting:   task.StatusCreated,
	api.StateRunning:    task.StatusRunning,
	api.StateStopping:   task.StatusRunning,
	api.StateRestarting: task.StatusRestarting,
	api.StateExited:     task.StatusExited,
}

// execution is the workload's own view of one of the service's runs: what it
// was asked to be, and what it has become.
func execution(run *api.Run) task.Execution {
	status := statuses[run.State]

	return task.Execution{
		ID:   run.ID,
		Name: run.Name,

		TaskUUID:    run.Task.UUID,
		TaskName:    run.Task.Name,
		Slug:        run.Task.Slug,
		Kind:        kindOf(task.Kind(run.Task.Kind)),
		NodeName:    run.Node,
		OwnerUUID:   run.Task.Owner,
		StackUUID:   run.Task.Stack,
		Attempt:     max(run.Task.Attempt, 0),
		Interactive: run.Task.Interactive,
		TTL:         time.Duration(max(run.Task.TTLSeconds, 0)) * time.Second,

		Status: status,
		Image:  run.Image,

		// the limits as they were asked for, which is not quite what the run
		// is held to: a VM is given whole CPUs, and no less memory than it
		// boots in.
		ResourceLimits: task.ResourceLimits{
			Cpu:    run.CPU,
			Memory: run.Memory,
			Disk:   run.Disk,
		},

		RestartPolicy:    run.RestartPolicy,
		RestartCount:     run.RestartCount,
		WorkingDirectory: run.WorkingDir,
		ExposedPorts:     exposedPorts(run, status),
		PortBindings:     portBindings(run, status),
		Networks:         network.Attachments(policy(run.Network), "", ""),
		Environment:      run.Environment,
		Entrypoint:       run.Entrypoint,
		Command:          run.Command,
		CreatedAt:        run.CreatedAt,
		StartedAt:        run.StartedAt,
		ExitCode:         run.ExitCode,
	}
}

// policy is the workload's network policy for one of the service's.
func policy(networkPolicy api.NetworkPolicy) network.Policy {
	if networkPolicy == api.NetworkPublic {
		return network.PolicyPublic
	}

	return network.PolicyIsolated
}

// exposedPorts and portBindings are a run's ports as docker reports a
// container's: while it runs, every port it publishes, bound at the host port
// the service picked, and nothing at all otherwise. That is what lets the
// heartbeat and the port proxy read them as they read a container's.
func exposedPorts(run *api.Run, status task.Status) port.PortSet {
	exposed := make(port.PortSet)
	if status != task.StatusRunning {
		return exposed
	}

	for _, p := range run.Ports {
		exposed[port.Port(p)] = struct{}{}
	}

	for _, endpoint := range run.Endpoints {
		exposed[port.Port(endpoint.Port)] = struct{}{}
	}

	return exposed
}

func portBindings(run *api.Run, status task.Status) port.PortMap {
	bindings := make(port.PortMap)
	if status != task.StatusRunning {
		return bindings
	}

	for _, endpoint := range run.Endpoints {
		if endpoint.HostPort == 0 {
			continue
		}

		bindings[port.Port(endpoint.Port)] = []port.PortBinding{{
			HostIP:   anyAddress,
			HostPort: port.Port(endpoint.HostPort),
		}}
	}

	return bindings
}

// newestFirst orders runs as docker lists containers, the latest first, so
// that what takes the first of a task's runs takes its latest attempt. Runs
// made in the same instant are told apart by their IDs, which the service
// makes in the order it makes the runs.
func newestFirst(a task.Execution, b task.Execution) int {
	if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
		return order
	}

	return strings.Compare(b.ID, a.ID)
}

// taskStats is what a run is using. Microsandbox does not count a guest's
// processes, so there are none to report.
func taskStats(stats api.Stats) task.Stats {
	return task.Stats{
		CPUPercent:    stats.CPUPercent,
		MemoryUsage:   stats.MemoryUsage,
		MemoryLimit:   stats.MemoryLimit,
		MemoryPercent: percent(stats.MemoryUsage, stats.MemoryLimit),
		NetworkInput:  stats.NetworkInput,
		NetworkOutput: stats.NetworkOutput,
		BlockInput:    stats.BlockInput,
		BlockOutput:   stats.BlockOutput,
	}
}

// nodeStats is what a node's running runs are using between them.
func nodeStats(stats api.Stats) node.Stats {
	return node.Stats{
		CPUPercent:    stats.CPUPercent,
		MemoryUsage:   stats.MemoryUsage,
		MemoryLimit:   stats.MemoryLimit,
		MemoryPercent: percent(stats.MemoryUsage, stats.MemoryLimit),
		NetworkInput:  stats.NetworkInput,
		NetworkOutput: stats.NetworkOutput,
		BlockInput:    stats.BlockInput,
		BlockOutput:   stats.BlockOutput,
	}
}

// percent is how much of a limit is used, as docker works it out: nothing at
// all when there is no limit.
func percent(usage uint64, limit uint64) float64 {
	if limit == 0 {
		return 0
	}

	return float64(usage) / float64(limit) * 100.0
}

// stream is which of a task's two streams a line came from. A line from a
// stream this client does not know is still kept, as output.
func stream(name string) task.Stream {
	if name == api.StreamStderr {
		return task.StreamStderr
	}

	return task.StreamStdout
}
