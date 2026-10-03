package microvm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"time"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	workloadRuntime "github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Runtime is a class's microVMs, as task.Runtime: a VM for every run, which
// vmhost holds. IDs are vmhost's own; putting the class in front is the
// multiplexer's.
//
// It keeps nothing. What a run is, is read back off vmhost each time, which
// keeps it with the VM as labels, so a Runtime made a moment ago knows every VM
// one made a year ago did.
//
// It acts only on its own node's VMs. Listings are asked for with the node's
// label, and anything vmhost hands back without it is dropped; a command for one
// VM is first checked against the VM's label, so an orchestrator sharing a
// vmhost with another can never stop, start or delete the other's VMs, even
// when asked to by ID. What only reads — a VM's output, its stats, a connection
// to its ports — is asked by IDs that came from those listings, and is not
// checked again: they are asked for several times a second.
type Runtime struct {
	client *Client
	node   string
	class  workloadRuntime.Class
	logger *slog.Logger
	tracer oteltrace.Tracer
}

var (
	_ task.Runtime = (*Runtime)(nil)
	_ task.Dialer  = (*Runtime)(nil)
)

// OnNode is every VM this node holds. Another node's are not this driver's to
// list, whatever vmhost holds.
func (r *Runtime) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	if nodeName != r.node {
		return nil, nil
	}

	return r.list(ctx, nodeNameLabel+"="+nodeName)
}

// Of is the VMs running one task on this node: the latest attempt, and
// whatever is left of the ones before it.
func (r *Runtime) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	return r.list(ctx, nodeNameLabel+"="+r.node, taskUUIDLabel+"="+taskUUID)
}

// BySlug is the VMs on this node answering to the name a task's ports are
// served under.
func (r *Runtime) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	return r.list(ctx, nodeNameLabel+"="+r.node, taskSlugLabel+"="+slug)
}

// list is how vmhost is asked all three of those: what a VM is running is
// written on it, so looking one up is looking at what it says.
func (r *Runtime) list(ctx context.Context, filters ...string) ([]task.Execution, error) {
	ctx, span := r.tracer.Start(ctx, "microvm.task.list",
		oteltrace.WithAttributes(attribute.StringSlice("labels", filters)),
	)
	defer span.End()

	vms, err := r.client.VMs(ctx, filters...)
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	executions := make([]task.Execution, 0, len(vms))
	for _, v := range vms {
		// vmhost filtered these already. One it handed back anyway is still
		// not this node's to act on, and handing it on is how it would be.
		if !v.Matches(filters) {
			continue
		}

		executions = append(executions, executionOf(v, r.class))
	}

	return executions, nil
}

// EnsureImage makes sure vmhost holds an image as a disk, pulling and
// converting it if it does not.
func (r *Runtime) EnsureImage(ctx context.Context, image string) error {
	ctx, span := r.tracer.Start(ctx, "microvm.image.ensure",
		oteltrace.WithAttributes(attribute.String("image", image)),
	)
	defer span.End()

	prepared, err := r.client.PrepareImage(ctx, image)
	if err != nil {
		return trace.RecordError(span, err)
	}

	span.SetAttributes(attribute.String("image.digest", prepared.Digest))

	return nil
}

// Create makes a VM for a run: its record and its disks. It boots nothing:
// Start does, as docker's create starts nothing.
func (r *Runtime) Create(ctx context.Context, execution *task.Execution) (string, error) {
	ctx, span := r.tracer.Start(ctx, "microvm.task.create",
		oteltrace.WithAttributes(attribute.String("image", execution.Image), attribute.String("name", execution.Name)),
	)
	defer span.End()

	spec, err := specOf(execution, r.node)
	if err != nil {
		return "", trace.RecordError(span, err)
	}

	id, err := r.client.Create(ctx, spec)
	if err != nil {
		return "", trace.RecordError(span, err)
	}

	r.logger.Info("vm created", "vm", id, "name", spec.Name, "image", spec.Image)

	return id, nil
}

// Start boots a VM and starts its task.
func (r *Runtime) Start(ctx context.Context, id string) error {
	return r.act(ctx, "microvm.task.start", id, func(ctx context.Context) error {
		r.logger.Info("starting vm", "vm", id)

		return r.client.Start(ctx, id)
	})
}

// Stop asks a VM's task to end, gives it the time docker gives a container,
// and then ends it. A VM that is not there any more is the outcome asked for,
// which the domain's own "not there" says.
func (r *Runtime) Stop(ctx context.Context, id string) error {
	return r.act(ctx, "microvm.task.stop", id, func(ctx context.Context) error {
		return r.client.Stop(ctx, id, vm.DefaultStopTimeout)
	})
}

// Restart stops a VM's task and starts it again, in a machine booted anew from
// the same disks: what the task wrote to its root survives, as it does a
// container's restart.
func (r *Runtime) Restart(ctx context.Context, id string) error {
	return r.act(ctx, "microvm.task.restart", id, func(ctx context.Context) error {
		return r.client.Restart(ctx, id)
	})
}

// Kill ends a VM's task at once, without the grace Stop gives it.
func (r *Runtime) Kill(ctx context.Context, id string) error {
	return r.act(ctx, "microvm.task.kill", id, func(ctx context.Context) error {
		return r.client.Kill(ctx, id)
	})
}

// Delete takes a VM away, with its disks and its output. One that runs is
// ended first; one that is not there is the outcome asked for.
func (r *Runtime) Delete(ctx context.Context, id string) error {
	return r.act(ctx, "microvm.task.delete", id, func(ctx context.Context) error {
		if err := r.client.Delete(ctx, id); err != nil {
			return err
		}

		r.logger.Info("vm deleted", "vm", id)

		return nil
	})
}

// act does one thing to one of this node's VMs.
func (r *Runtime) act(ctx context.Context, name string, id string, do func(context.Context) error) error {
	ctx, span := r.tracer.Start(ctx, name, oteltrace.WithAttributes(attribute.String("task.id", id)))
	defer span.End()

	if _, err := r.own(ctx, id); err != nil {
		return trace.RecordError(span, err)
	}

	return trace.RecordError(span, do(ctx))
}

// own is one VM, if it is this node's. One that is another node's is, as far
// as this node is concerned, not there: vm.ErrNotFound, which the domain reads
// as already gone.
func (r *Runtime) own(ctx context.Context, id string) (vm.VM, error) {
	found, err := r.client.VM(ctx, id)
	if err != nil {
		return vm.VM{}, err
	}

	if found.Spec.Labels[nodeNameLabel] != r.node {
		return vm.VM{}, fmt.Errorf("%w: vm %s is not node %q's", vm.ErrNotFound, id, r.node)
	}

	return found, nil
}

// Inspect is one of this node's VMs as a run.
func (r *Runtime) Inspect(ctx context.Context, id string) (task.Execution, error) {
	ctx, span := r.tracer.Start(ctx, "microvm.task.inspect",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	found, err := r.own(ctx, id)
	if err != nil {
		return task.Execution{}, trace.RecordError(span, err)
	}

	return executionOf(found, r.class), nil
}

// Stats is what a VM uses. One that does not run uses nothing, as a stopped
// container does, rather than failing to say so.
func (r *Runtime) Stats(ctx context.Context, id string) (task.Stats, error) {
	ctx, span := r.tracer.Start(ctx, "microvm.task.stats",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	stats, err := r.client.Stats(ctx, id)
	if errors.Is(err, vm.ErrNotRunning) {
		return task.Stats{}, nil
	}

	if err != nil {
		return task.Stats{}, trace.RecordError(span, err)
	}

	return statsOf(stats), nil
}

// Logs writes a VM's whole output so far, both streams in the order they were
// written, one line at a time.
func (r *Runtime) Logs(ctx context.Context, id string, writer io.Writer) error {
	ctx, span := r.tracer.Start(ctx, "microvm.task.logs",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	err := r.client.Logs(ctx, id, 0, time.Time{}, false, func(line vm.LogLine) error {
		_, err := io.WriteString(writer, line.Content+"\n")

		return err
	})

	return trace.RecordError(span, err)
}

// StreamLogs follows a VM's output from since onward, handing each line to emit
// as it arrives. It returns when vmhost ends the stream, when emit refuses a
// line, or when ctx is done — which is how a caller stops following, and is
// not a failure.
//
// Lines carry the time the guest wrote them, so a caller that resumes from the
// last one it saw reads that one again rather than missing any, as it does
// with docker.
func (r *Runtime) StreamLogs(ctx context.Context, id string, since time.Time, emit func(task.LogLine) error) error {
	ctx, span := r.tracer.Start(ctx, "microvm.task.logs.stream",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	err := r.client.Logs(ctx, id, 0, since, true, func(line vm.LogLine) error {
		return emit(task.LogLine{Stream: streamOf(line.Stream), Content: line.Content, At: line.At})
	})

	if ctx.Err() != nil {
		return nil
	}

	return trace.RecordError(span, err)
}

// Exec starts a command inside a running VM and hands back the stream it runs
// on. Closing the session releases that stream; ending it is what stops the
// command.
func (r *Runtime) Exec(ctx context.Context, id string, options task.ExecOptions) (task.ExecSession, error) {
	ctx, span := r.tracer.Start(ctx, "microvm.task.exec",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	if _, err := r.own(ctx, id); err != nil {
		return nil, trace.RecordError(span, err)
	}

	execID, conn, err := r.client.Exec(ctx, id, guest.Exec{
		Process: guest.Process{Args: options.Command, Env: options.Env, WorkingDir: options.WorkDir},
		TTY:     options.TTY,
	})
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	r.logger.Info("exec started", "vm", id, "execID", execID)

	return &execSession{client: r.client, vmID: id, execID: execID, conn: conn}, nil
}

// DialContext connects to one of a VM's ports through vmhost, which reaches it
// through the VM's agent: nothing on the host has a route into a VM's network,
// which is the point of it. The context bounds connecting, not the connection.
func (r *Runtime) DialContext(ctx context.Context, executionID string, p port.Port) (net.Conn, error) {
	ctx, span := r.tracer.Start(ctx, "microvm.task.dial",
		oteltrace.WithAttributes(attribute.String("task.id", executionID), attribute.Int64("task.port", int64(p))),
	)
	defer span.End()

	if p == 0 || p > math.MaxUint16 {
		return nil, trace.RecordError(span, fmt.Errorf("%w: %d is not a port", vm.ErrInvalid, p))
	}

	conn, err := r.client.Dial(ctx, executionID, uint16(p))
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	return conn, nil
}
