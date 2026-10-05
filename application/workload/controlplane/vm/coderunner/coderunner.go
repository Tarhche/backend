// Package coderunner shows the code runner's runs among the VMs.
//
// The code runner runs each snippet as a task of the guest's, in a VM of its
// own for as long as the snippet runs, and the task is taken away once the
// snippet has ended. The task is the one record of a run: nothing is written
// down as a VM, and the VM shown is the task, read as one (VM). Only a listing
// of anybody's VMs has them, since no user's uuid is the guest's.
//
// A run can be stopped, deleted and read, which is stopping its task, deleting
// it and reading what it wrote. Everything else a VM can be asked to do is
// refused as CodeRefused: a run is the code runner's to start, and it is gone
// once it has ended.
package coderunner

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	deletetask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// CodeRefused is what a run is refused with when it is asked to be started,
	// restarted, changed, restored or snapshotted, which only a VM somebody
	// asked for can be.
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

func New(tasks task.Repository, producer domain.Producer, remove *deletetask.UseCase) *Runs {
	return &Runs{tasks: tasks, producer: producer, remove: remove}
}

// All is every run there is now, as VMs, newest first.
func (r *Runs) All(ctx context.Context) ([]vm.VM, error) {
	var runs []vm.VM

	for offset := uint(0); ; offset += batch {
		read, err := r.tasks.GetAllByOwner(ctx, task.GuestOwnerUUID, offset, batch)
		if err != nil {
			return nil, err
		}

		for i := range read {
			runs = append(runs, VM(&read[i]))
		}

		if uint(len(read)) < batch {
			break
		}
	}

	slices.SortStableFunc(runs, newestFirst)

	return runs, nil
}

// One is the run uuid names, as its task, to whoever may see anybody's VMs.
// Somebody asking after their own has none, since a run is nobody's own: it is
// domain.ErrNotExists to them, as a run that is not there is to anybody.
func (r *Runs) One(ctx context.Context, ownerUUID string, uuid string) (task.Task, error) {
	if len(ownerUUID) > 0 || len(uuid) == 0 {
		return task.Task{}, domain.ErrNotExists
	}

	return r.tasks.GetOneByOwner(ctx, task.GuestOwnerUUID, uuid)
}

// Refused is what a VM's use case answers with when the VM it was asked about
// could not be read, err being what reading it came to: a uuid that names a
// run is refused as CodeRefused, and anything else is handed back as the error
// it was.
func (r *Runs) Refused(ctx context.Context, ownerUUID string, uuid string, err error) (domain.ValidationErrors, error) {
	if !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	if _, runErr := r.One(ctx, ownerUUID, uuid); errors.Is(runErr, domain.ErrNotExists) {
		return nil, err
	} else if runErr != nil {
		return nil, runErr
	}

	return domain.ValidationErrors{"vm": CodeRefused}, nil
}

// Stop asks for a run to be stopped, which is asking its task to stop.
//
// What is wanted of it is written down before its node is asked, whatever
// comes of asking, as it is for any task; one that cannot get there from where
// it is, such as one stopping already, is refused as invalid_state_transition.
// Its node stops its VM, and a run that has ended is taken away with it.
func (r *Runs) Stop(ctx context.Context, t *task.Task) (domain.ValidationErrors, error) {
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

// Delete takes a run away, running or not: its task and its log go, and its
// node removes its VM. One that went in the meantime is gone already, which is
// what was asked for.
func (r *Runs) Delete(ctx context.Context, t *task.Task) error {
	_, err := r.remove.Execute(ctx, &deletetask.Request{UUID: t.UUID, Force: true})
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return err
}

// Logs is what a run has written, as the lines of a VM's log, and whether
// there were more of them than a reply carries.
//
// A run is a job, and a job's whole output rides every heartbeat its node
// sends and is kept on its task. Its lines have no time to them, so each is
// given the moment the run was made, a nanosecond after the line before it:
// that keeps them in the order they were written, keeps two that say the same
// thing apart, and lets whoever is following the log ask for what came after
// the last line they read, as they would of a VM's. Since leaves out the lines
// before it, and the last Tail are kept, or as many as a reply carries.
func Logs(t *task.Task, options vm.LogOptions) ([]noderequest.VMLogLine, bool) {
	lines := make([]noderequest.VMLogLine, 0)

	output := strings.TrimSuffix(string(t.ExecutionLogs), "\n")
	if len(output) == 0 {
		return lines, false
	}

	for i, line := range strings.Split(output, "\n") {
		at := t.CreatedAt.Add(time.Duration(i))
		if at.Before(options.Since) {
			continue
		}

		lines = append(lines, noderequest.VMLogLine{At: at, Source: vm.LogSourceMain, Line: strings.TrimSuffix(line, "\r")})
	}

	tail, carried := options.Tail, false
	if tail == 0 || tail > noderequest.MaxLogLines {
		tail, carried = noderequest.MaxLogLines, true
	}

	if uint(len(lines)) <= tail {
		return lines, false
	}

	return lines[uint(len(lines))-tail:], carried
}

// VM is a run as the VM it runs in.
//
// It is its task's uuid, name and slug, and the guest's: a machine booted from
// the runner's image, given what its task was limited to the way a node gives
// it, with the network its task's policy maps to and a disk thrown away with
// it. Its lifetime is its task's ttl, counted from when it came up, and its
// state is its task's, in a VM's words. Its node reports no stats for a run, so
// it has none to show.
func VM(t *task.Task) vm.VM {
	return vm.VM{
		UUID:            t.UUID,
		Name:            t.Name,
		Slug:            t.Slug,
		OwnerUUID:       t.OwnerUUID,
		Kind:            vm.KindMachine,
		Image:           t.Image,
		Resources:       t.ResourceLimits.VMResources(),
		Ports:           portsOf(t),
		Network:         t.NetworkPolicy.VMNetwork(),
		PersistentDisk:  false,
		Lifetime:        t.TTL,
		ExpiresAt:       expiresAt(t),
		CurrentState:    stateOf(t.CurrentState),
		ExpectedState:   stateOf(t.ExpectedState),
		Reason:          t.Reason,
		NodeName:        t.NodeName,
		LastHeartbeatAt: t.LastHeartbeatAt,
		CreatedAt:       t.CreatedAt,
		StartedAt:       t.StartedAt,
		UpdatedAt:       latest(t.CreatedAt, t.StartedAt, t.FinishedAt),
		ManagedBy:       vm.ManagedByCodeRunner,
	}
}

// Merge is VMs and runs in one listing, newest first, as each of them is
// listed already: by when they were made, and by uuid for those made at the
// same moment, as the stores order them.
func Merge(vms []vm.VM, runs []vm.VM) []vm.VM {
	merged := make([]vm.VM, 0, len(vms)+len(runs))

	for len(vms) > 0 && len(runs) > 0 {
		if newestFirst(runs[0], vms[0]) < 0 {
			merged, runs = append(merged, runs[0]), runs[1:]
		} else {
			merged, vms = append(merged, vms[0]), vms[1:]
		}
	}

	merged = append(merged, vms...)

	return append(merged, runs...)
}

// newestFirst orders VMs by when they were made, the newest first. A uuid is a
// v7, which orders by when it was made too, so of two made at the same moment
// the one whose uuid sorts last goes first.
func newestFirst(a vm.VM, b vm.VM) int {
	if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
		return order
	}

	return strings.Compare(b.UUID, a.UUID)
}

// stateOf is a task's state in a VM's words. A job that ran to its end is a VM
// that has stopped.
func stateOf(state task.State) vm.State {
	switch state {
	case task.Created:
		return vm.Created
	case task.Scheduled:
		return vm.Scheduled
	case task.Running:
		return vm.Running
	case task.Stopping:
		return vm.Stopping
	case task.Stopped, task.Completed:
		return vm.Stopped
	case task.Failed:
		return vm.Failed
	case task.Restarting:
		return vm.Restarting
	default:
		return 0
	}
}

// expiresAt is when a run is stopped for having run long enough: the deadline
// its node set as it came up, which is its ttl counted from when it started.
// One that has not come up yet has none, and neither has one that may run for
// as long as it likes.
func expiresAt(t *task.Task) time.Time {
	if !t.Deadline.IsZero() {
		return t.Deadline
	}

	if t.TTL > 0 && !t.StartedAt.IsZero() {
		return t.StartedAt.Add(t.TTL)
	}

	return time.Time{}
}

// portsOf is the ports a run serves, once each and lowest first, as its VM is
// given them.
func portsOf(t *task.Task) []port.Port {
	ports := slices.Clone(t.ExposedPorts)

	for _, bindings := range t.PortBindings {
		for p := range bindings {
			ports = append(ports, p)
		}
	}

	if ports == nil {
		return []port.Port{}
	}

	slices.Sort(ports)

	return slices.Compact(ports)
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
