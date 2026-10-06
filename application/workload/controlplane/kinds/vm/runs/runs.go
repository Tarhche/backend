// Package runs shows the code runner's runs among anybody's VMs, as the vm
// kind's extras (kind.Extras).
//
// The code runner runs each snippet as a task of the guest's, in a VM of its
// own for as long as the snippet runs, and the task is taken away once the
// snippet has ended. The task is the one record of a run: nothing is kept as
// a VM, and the VM shown is the task, read as one, labelled as managed by the
// code runner. Only a listing of anybody's VMs has them, since no user's uuid
// is the guest's, and nobody's own listing ever does.
//
// A run can be stopped, deleted and read, which is stopping its task,
// deleting it and reading what it wrote. Everything else a VM can be asked
// to do is refused as CodeRefused, under vm: a run is the code runner's to
// start, and it is gone once it has ended.
package runs

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	deletetask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// CodeRefused is what a run is refused with when it is asked to be
	// started, restarted, changed or restored, which only a VM somebody asked
	// for can be.
	CodeRefused = "managed_by_code_runner"

	// batch is how many runs are read at a time.
	batch uint = 100
)

// Runs are the code runner's runs, read from its tasks.
type Runs struct {
	tasks    task.Repository
	producer domain.Producer

	// remove takes a task away the one way a task is taken away, with its
	// log, which is how the code runner takes a run away when its reader
	// stops it.
	remove *deletetask.UseCase
}

var _ kind.Extras = &Runs{}

func New(tasks task.Repository, producer domain.Producer, remove *deletetask.UseCase) *Runs {
	return &Runs{tasks: tasks, producer: producer, remove: remove}
}

// All is every run there is now, as VMs, newest first.
func (r *Runs) All(ctx context.Context) ([]kind.Raw, error) {
	var runs []kind.Raw

	for offset := uint(0); ; offset += batch {
		read, err := r.tasks.GetAllByOwner(ctx, task.GuestOwnerUUID, offset, batch)
		if err != nil {
			return nil, err
		}

		for i := range read {
			run, err := manifest(&read[i])
			if err != nil {
				return nil, err
			}

			runs = append(runs, run)
		}

		if uint(len(read)) < batch {
			break
		}
	}

	slices.SortStableFunc(runs, newestFirst)

	return runs, nil
}

// One is the run uuid names, as the VM it runs in, or domain.ErrNotExists.
func (r *Runs) One(ctx context.Context, uuid string) (kind.Raw, error) {
	t, err := r.task(ctx, uuid)
	if err != nil {
		return kind.Raw{}, err
	}

	return manifest(&t)
}

// Act asks a run for what a VM is asked: a stop stops its task, a delete
// takes its task away, and anything else is refused as CodeRefused.
func (r *Runs) Act(ctx context.Context, run kind.Raw, action string, _ []byte) (kind.Raw, bool, domain.ValidationErrors, error) {
	t, err := r.task(ctx, run.Metadata.UUID)
	if err != nil {
		return kind.Raw{}, false, nil, err
	}

	switch action {
	case vmKind.ActionStop:
		refused, err := r.stop(ctx, &t)
		if err != nil || len(refused) > 0 {
			return kind.Raw{}, false, refused, err
		}

		after, err := manifest(&t)

		return after, false, nil, err

	case vmKind.ActionDelete:
		return kind.Raw{}, true, nil, r.delete(ctx, &t)
	}

	return kind.Raw{}, false, refusal(), nil
}

// Query reads what a run has written, as a VM's log, and refuses anything
// else as CodeRefused.
func (r *Runs) Query(ctx context.Context, run kind.Raw, action string, payload []byte) ([]byte, domain.ValidationErrors, error) {
	if action != vmKind.ActionLogs {
		return nil, refusal(), nil
	}

	var options vmKind.LogsPayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &options); err != nil {
			return nil, domain.ValidationErrors{"payload": "invalid_value"}, nil
		}
	}

	t, err := r.task(ctx, run.Metadata.UUID)
	if err != nil {
		return nil, nil, err
	}

	answer, err := json.Marshal(Logs(&t, options))

	return answer, nil, err
}

// task is the run uuid names, as its task.
func (r *Runs) task(ctx context.Context, uuid string) (task.Task, error) {
	if len(uuid) == 0 {
		return task.Task{}, domain.ErrNotExists
	}

	return r.tasks.GetOneByOwner(ctx, task.GuestOwnerUUID, uuid)
}

// stop asks for a run to be stopped, which is asking its task to stop.
//
// What is wanted of it is written down before its node is asked, whatever
// comes of asking, as it is for any task; one that cannot get there from
// where it is, such as one stopping already, is refused as
// invalid_state_transition. Its node stops its VM, and a run that has ended
// is taken away with it.
func (r *Runs) stop(ctx context.Context, t *task.Task) (domain.ValidationErrors, error) {
	t.ExpectedState = task.Stopped

	if !task.ValidStateTransition(t.CurrentState, task.Stopping) {
		if _, err := r.tasks.Save(ctx, t); err != nil {
			return nil, err
		}

		return domain.ValidationErrors{"vm": "invalid_state_transition"}, nil
	}

	t.CurrentState = task.Stopping
	if _, err := r.tasks.Save(ctx, t); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(events.TaskStoppageRequested{UUID: t.UUID})
	if err != nil {
		return nil, err
	}

	return nil, r.producer.Produce(context.WithoutCancel(ctx), events.TaskStoppageRequestedName, payload)
}

// delete takes a run away, running or not: its task and its log go, and its
// node removes its VM. One that went in the meantime is gone already, which
// is what was asked for.
func (r *Runs) delete(ctx context.Context, t *task.Task) error {
	_, err := r.remove.Execute(ctx, &deletetask.Request{UUID: t.UUID, Force: true})
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return err
}

// refusal is what a run is refused anything but a stop, a delete and a read
// with.
func refusal() domain.ValidationErrors {
	return domain.ValidationErrors{"vm": CodeRefused}
}

// Logs is what a run has written, as the lines of a VM's log, and whether
// there were more of them than an answer carries.
//
// A run is a job, and a job's whole output rides every heartbeat its node
// sends and is kept on its task. Its lines have no time to them, so each is
// given the moment the run was made, a nanosecond after the line before it:
// that keeps them in the order they were written, keeps two that say the same
// thing apart, and lets whoever is following the log ask for what came after
// the last line they read, as they would of a VM's. Since leaves out the
// lines before it, and the last Tail are kept, or as many as an answer
// carries.
func Logs(t *task.Task, options vmKind.LogsPayload) vmKind.Logs {
	lines := make([]vmKind.LogLine, 0)

	output := strings.TrimSuffix(string(t.ExecutionLogs), "\n")
	if len(output) == 0 {
		return vmKind.Logs{Lines: lines}
	}

	for i, line := range strings.Split(output, "\n") {
		at := t.CreatedAt.Add(time.Duration(i))
		if at.Before(options.Since) {
			continue
		}

		lines = append(lines, vmKind.LogLine{At: at, Source: vm.LogSourceMain, Line: strings.TrimSuffix(line, "\r")})
	}

	tail, carried := options.Tail, false
	if tail == 0 || tail > noderequest.MaxLogLines {
		tail, carried = noderequest.MaxLogLines, true
	}

	if uint(len(lines)) <= tail {
		return vmKind.Logs{Lines: lines}
	}

	return vmKind.Logs{Lines: lines[uint(len(lines))-tail:], Truncated: carried}
}

// Manifest is a run as the VM it runs in.
//
// It is its task's uuid, name and slug, and the guest's: a machine booted
// from the runner's image, given what its task was limited to the way a node
// gives it, with the network its task's policy maps to and a disk thrown away
// with it. Its lifetime is its task's ttl, counted from when it came up, and
// its state is its task's, in a VM's words. Its node reports no stats for a
// run, so it has none to show.
func Manifest(t *task.Task) vmKind.VM {
	return vmKind.VM{
		Kind: vmKind.Name,
		Metadata: kind.Metadata{
			UUID:      t.UUID,
			Name:      t.Name,
			Slug:      t.Slug,
			OwnerUUID: t.OwnerUUID,
			Labels: map[string]string{
				vmKind.LabelFlavor:    string(vmKind.FlavorMachine),
				vmKind.LabelManagedBy: vmKind.ManagedByCodeRunner,
			},
			Node:      t.NodeName,
			Lifetime:  t.TTL,
			ExpiresAt: expiresAt(t),
			CreatedAt: t.CreatedAt,
			UpdatedAt: latest(t.CreatedAt, t.StartedAt, t.FinishedAt),
		},
		Spec: vmKind.Spec{
			Flavor:    vmKind.FlavorMachine,
			Image:     t.Image,
			Resources: vmKind.ResourcesOf(t.ResourceLimits.VMResources()),
			Ports:     portsOf(t),
			Network:   networkOf(t.NetworkPolicy.VMNetwork()),
		},
		Status: vmKind.Status{
			Status: kind.Status{
				State:      stateOf(t.CurrentState),
				Expected:   stateOf(t.ExpectedState),
				Reason:     t.Reason,
				ObservedAt: t.LastHeartbeatAt,
			},
			StartedAt: t.StartedAt,
		},
	}
}

func manifest(t *task.Task) (kind.Raw, error) {
	return kind.Encode(Manifest(t))
}

// networkOf is a task's network as a VM's spec has it.
func networkOf(n vm.Network) vmKind.Network {
	return vmKind.Network{Ingress: n.Ingress, Egress: n.Egress}
}

// newestFirst orders runs by when they were made, the newest first. A uuid
// is a v7, which orders by when it was made too, so of two made at the same
// moment the one whose uuid sorts last goes first.
func newestFirst(a kind.Raw, b kind.Raw) int {
	if order := b.Metadata.CreatedAt.Compare(a.Metadata.CreatedAt); order != 0 {
		return order
	}

	return strings.Compare(b.Metadata.UUID, a.Metadata.UUID)
}

// stateOf is a task's state in a VM's words. A job that ran to its end is a
// VM that has stopped.
func stateOf(state task.State) kind.State {
	switch state {
	case task.Created:
		return vmKind.Created
	case task.Scheduled:
		return vmKind.Scheduled
	case task.Running:
		return vmKind.Running
	case task.Stopping:
		return vmKind.Stopping
	case task.Stopped, task.Completed:
		return vmKind.Stopped
	case task.Failed:
		return vmKind.Failed
	case task.Restarting:
		return vmKind.Restarting
	default:
		return ""
	}
}

// expiresAt is when a run is stopped for having run long enough: the
// deadline its node set as it came up, which is its ttl counted from when it
// started. One that has not come up yet has none, and neither has one that
// may run for as long as it likes.
func expiresAt(t *task.Task) time.Time {
	if !t.Deadline.IsZero() {
		return t.Deadline
	}

	if t.TTL > 0 && !t.StartedAt.IsZero() {
		return t.StartedAt.Add(t.TTL)
	}

	return time.Time{}
}

// portsOf is the ports a run serves, once each and lowest first, as its VM
// is given them.
func portsOf(t *task.Task) []port.Port {
	ports := slices.Clone(t.ExposedPorts)

	for _, bindings := range t.PortBindings {
		for p := range bindings {
			ports = append(ports, p)
		}
	}

	return vmKind.Normalized(ports)
}

// latest is the last of some moments, leaving out those that have not come.
func latest(moments ...time.Time) time.Time {
	var last time.Time

	for _, moment := range moments {
		if moment.After(last) {
			last = moment
		}
	}

	return last
}
