// Package vmruntime runs the code runner's tasks in VMs: one ephemeral VM per
// execution, on the node's engine.
//
// A task is what it was when it ran in a container: an image, a command, a
// network policy, ports and limits. Each run of it is a VM of its own, booted
// from the task's image with the task's command as its main process. The VM
// stops when the process exits, keeping its exit code and its log, and it
// shares no disk and no network with anything else, which a container on a
// shared daemon could not promise.
package vmruntime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"slices"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// executions is the namespace an execution's id is made in: one per task and
// attempt, the same each time it is asked for, so a task asked to run twice
// finds the run it already has rather than making a second.
var executions = uuid.Must(uuid.FromString("74083341-0030-431a-9fa4-97baee9ebd2d"))

// killedBySignal is the exit code a process killed by a signal it was not
// told is given, as a shell gives one killed by SIGKILL. An engine that says
// a process ended without saying how it exited says it with a negative code.
const killedBySignal = 128 + 9

// pollInterval is how often a running task's log is read for what it wrote
// since, which is how it is followed: an engine keeps a log and answers for
// it, and has no stream to follow.
const pollInterval = 500 * time.Millisecond

// Runtime runs tasks on a node's engine.
type Runtime struct {
	engine vm.Engine
	logger *slog.Logger
	poll   time.Duration
}

var _ task.Runtime = &Runtime{}

func New(engine vm.Engine, logger *slog.Logger) *Runtime {
	return &Runtime{engine: engine, logger: logger, poll: pollInterval}
}

// OnNode is every run the named node is holding: the engine's instances that
// run tasks for it.
func (r *Runtime) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	return r.matching(ctx, func(labels map[string]string) bool {
		node := labels[labelNode]

		return len(node) == 0 || node == nodeName
	})
}

// Of is the runs of one task, latest attempt and whatever is left of the ones
// before it.
func (r *Runtime) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	return r.matching(ctx, func(labels map[string]string) bool {
		return labels[vm.LabelTask] == taskUUID
	})
}

// BySlug is the runs answering to the name a task's ports are served under.
func (r *Runtime) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	return r.matching(ctx, func(labels map[string]string) bool {
		return labels[vm.LabelSlug] == slug
	})
}

// matching is the runs whose labels say match: what an instance is running is
// written on it, so looking one up is looking at what it says.
func (r *Runtime) matching(ctx context.Context, match func(labels map[string]string) bool) ([]task.Execution, error) {
	instances, err := r.engine.List(ctx)
	if err != nil {
		return nil, err
	}

	executions := make([]task.Execution, 0, len(instances))
	for _, instance := range instances {
		if !isTask(instance.Labels) || !match(instance.Labels) {
			continue
		}

		executions = append(executions, executionOf(instance))
	}

	return executions, nil
}

// EnsureImage does nothing: the engine pulls a VM's image when it creates the
// VM, and a VM is all a task runs in.
func (r *Runtime) EnsureImage(context.Context, string) error {
	return nil
}

// Create makes the VM a run of a task is, and boots it, which starts its
// command.
//
// The VM is given the task's limits: whole vCPUs, rounded up from the cores it
// asked for, and its memory and disk in bytes, as they were asked for. Its
// network is what the task's policy maps to, and its ports are published only
// when that lets anything reach it. Its disk is thrown away with it.
func (r *Runtime) Create(ctx context.Context, execution *task.Execution) (string, error) {
	spec := vm.Spec{
		ID:    executionID(execution),
		Kind:  vm.KindMachine,
		Image: execution.Image,
		Resources: vm.Resources{
			CPUs:   cpus(execution.ResourceLimits.Cpu),
			Memory: execution.ResourceLimits.Memory,
			Disk:   execution.ResourceLimits.Disk,
		},
		Ports:      portsOf(execution),
		Network:    execution.NetworkPolicy.VMNetwork(),
		Labels:     labelsOf(execution),
		Entrypoint: slices.Clone(execution.Entrypoint),
		Command:    slices.Clone(execution.Command),
		Env:        slices.Clone(execution.Environment),
		WorkingDir: execution.WorkingDirectory,
	}

	created, err := r.engine.Create(ctx, spec)
	if err != nil {
		return "", err
	}

	r.logger.Info("task vm created", "execution", created.ID, "task", execution.TaskUUID, "image", execution.Image)

	return created.ID, nil
}

// Start boots a run that is down. One that is running is what was asked for,
// and one whose command has run to its end is not run again: a task asked for
// again is a new attempt, which is a new run.
func (r *Runtime) Start(ctx context.Context, executionID string) error {
	instance, err := r.engine.Inspect(ctx, executionID)
	if err != nil {
		return err
	}

	switch instance.State {
	case vm.InstanceRunning, vm.InstanceExited:
		return nil
	default:
		return r.engine.Start(ctx, executionID)
	}
}

func (r *Runtime) Stop(ctx context.Context, executionID string) error {
	return r.engine.Stop(ctx, executionID)
}

// Restart boots a run again in place, which runs its command again.
func (r *Runtime) Restart(ctx context.Context, executionID string) error {
	return r.engine.Restart(ctx, executionID)
}

// Kill stops a run. A VM has no grace period to cut short: stopping one ends
// it at once.
func (r *Runtime) Kill(ctx context.Context, executionID string) error {
	return r.engine.Stop(ctx, executionID)
}

// Delete removes a run, disk and all. One that is not there is gone already.
func (r *Runtime) Delete(ctx context.Context, executionID string) error {
	return r.engine.Delete(ctx, executionID)
}

func (r *Runtime) Inspect(ctx context.Context, executionID string) (task.Execution, error) {
	instance, err := r.engine.Inspect(ctx, executionID)
	if err != nil {
		return task.Execution{}, err
	}

	if !isTask(instance.Labels) {
		return task.Execution{}, fmt.Errorf("%w: %q is not a task's", domain.ErrNotExists, executionID)
	}

	return executionOf(instance), nil
}

// Stats samples what a run is using. Its CPU percent is the engine's: 0 to 100
// of all of the VM's vCPUs together.
func (r *Runtime) Stats(ctx context.Context, executionID string) (task.Stats, error) {
	stats, err := r.engine.Stats(ctx, executionID)
	if err != nil {
		return task.Stats{}, err
	}

	sampled := task.Stats{
		CPUPercent:    stats.CPUPercent,
		MemoryUsage:   stats.MemoryUsed,
		MemoryLimit:   stats.MemoryLimit,
		NetworkInput:  stats.NetworkRx,
		NetworkOutput: stats.NetworkTx,
	}

	if stats.MemoryLimit > 0 {
		sampled.MemoryPercent = float64(stats.MemoryUsed) / float64(stats.MemoryLimit) * 100
	}

	return sampled, nil
}

// Logs writes what a run's command wrote, a line at a time. What the kernel
// and the engine say while the VM boots is the VM's, not the task's.
func (r *Runtime) Logs(ctx context.Context, executionID string, writer io.Writer) error {
	lines, err := r.engine.Logs(ctx, executionID, vm.LogOptions{})
	if err != nil {
		return err
	}

	for _, line := range lines {
		if line.Source != vm.LogSourceMain {
			continue
		}

		if _, err := io.WriteString(writer, line.Line+"\n"); err != nil {
			return err
		}
	}

	return nil
}

// StreamLogs follows what a run's command writes from since onward, handing
// each line to emit as it is read. It returns once the run is no longer
// running and the last of what it wrote has been read, when emit refuses a
// line, or when ctx is done, which is how a caller stops following.
//
// The log is read again every poll for what was written since the last line
// read. The lines written at that very moment come back each time, and are
// handed on once.
func (r *Runtime) StreamLogs(ctx context.Context, executionID string, since time.Time, emit func(task.LogLine) error) error {
	ticker := time.NewTicker(r.poll)
	defer ticker.Stop()

	// handed is how many lines written at the moment since names were handed
	// on already.
	handed := 0

	for {
		// asked before the log is read, so that the read after it ended is
		// the last there is.
		instance, inspectErr := r.engine.Inspect(ctx, executionID)

		lines, err := r.engine.Logs(ctx, executionID, vm.LogOptions{Since: since})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return err
		}

		skip := handed
		for _, line := range lines {
			if line.Source != vm.LogSourceMain || line.At.Before(since) {
				continue
			}

			if line.At.After(since) {
				// a later moment: nothing of it has been handed on yet.
				since, handed, skip = line.At, 0, 0
			} else if skip > 0 {
				skip--

				continue
			}

			handed++

			if err := emit(task.LogLine{Stream: task.StreamStdout, Content: line.Line, At: line.At}); err != nil {
				return err
			}
		}

		if inspectErr != nil || instance.State != vm.InstanceRunning {
			return nil
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Exec runs a command inside a running run, which is what a live snippet's
// terminal is.
func (r *Runtime) Exec(ctx context.Context, executionID string, options task.ExecOptions) (task.ExecSession, error) {
	session, err := r.engine.Exec(ctx, executionID, vm.ExecOptions{
		Command:    options.Command,
		TTY:        options.TTY,
		Env:        options.Env,
		WorkingDir: options.WorkDir,
	})
	if err != nil {
		return nil, err
	}

	return newExecSession(session, options.TTY), nil
}

// executionID is the engine's name for one run of a task: the same for the
// same task and attempt, so a create asked for twice is refused the second
// time rather than making a run nobody follows.
func executionID(execution *task.Execution) string {
	if len(execution.TaskUUID) == 0 {
		return uuid.Must(uuid.NewV7()).String()
	}

	return uuid.NewV5(executions, execution.TaskUUID+"/"+strconv.Itoa(execution.Attempt)).String()
}

// cpus is the whole vCPUs a run is given for the cores it asked for, which is
// never fewer than one.
func cpus(cores float64) uint {
	return uint(max(math.Ceil(cores), 1))
}

// portsOf is the ports a run exposes, once each and lowest first.
func portsOf(execution *task.Execution) []port.Port {
	ports := make([]port.Port, 0, len(execution.ExposedPorts)+len(execution.PortBindings))

	for p := range execution.ExposedPorts {
		ports = append(ports, p)
	}

	for p := range execution.PortBindings {
		ports = append(ports, p)
	}

	slices.Sort(ports)

	return slices.Compact(ports)
}

// executionOf is a run as the engine sees it.
func executionOf(instance vm.Instance) task.Execution {
	execution := task.Execution{
		ID:           instance.ID,
		Status:       statusOf(instance.State),
		StartedAt:    instance.StartedAt,
		ExitCode:     exitCodeOf(instance),
		ExposedPorts: make(port.PortSet, len(instance.Endpoints)),
		PortBindings: make(port.PortMap, len(instance.Endpoints)),
	}

	identify(&execution, instance.Labels)

	execution.Name = execution.Slug
	if len(execution.Name) == 0 {
		execution.Name = instance.ID
	}

	// where each published port is reached, which is the engine's to say and
	// changes when the run does: the host is the node's own vmhost, and the
	// port the one it gave.
	for _, endpoint := range instance.Endpoints {
		host, published, err := net.SplitHostPort(endpoint.Address)
		if err != nil {
			continue
		}

		hostPort, err := strconv.ParseUint(published, 10, 16)
		if err != nil {
			continue
		}

		execution.ExposedPorts[endpoint.Port] = struct{}{}

		// a run that is not running serves nothing on it, as a stopped
		// container publishes nothing.
		if instance.State == vm.InstanceRunning {
			execution.PortBindings[endpoint.Port] = []port.PortBinding{{HostIP: host, HostPort: port.Port(hostPort)}}
		}
	}

	return execution
}

// statusOf is what an instance's state is as a run's status. A run that was
// stopped from outside has ended, as one whose command exited has.
func statusOf(state vm.InstanceState) task.Status {
	switch state {
	case vm.InstanceCreated:
		return task.StatusCreated
	case vm.InstanceRunning:
		return task.StatusRunning
	case vm.InstanceStopped, vm.InstanceExited:
		return task.StatusExited
	default:
		return task.StatusDead
	}
}

// exitCodeOf is what a run's command returned, when it has ended. An engine
// that says a process was killed without saying by what gives a negative code,
// which is read as the signal a shell would report: a run that was cut short
// rather than one that failed.
func exitCodeOf(instance vm.Instance) int {
	if instance.State != vm.InstanceExited {
		return 0
	}

	if instance.ExitCode < 0 {
		return killedBySignal
	}

	return instance.ExitCode
}
