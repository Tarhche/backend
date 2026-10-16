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
// A run is what any VM is to whoever reads the dashboard, but for what only a
// VM somebody asked for can be. It can be stopped and deleted, which is
// asking its task for its stop and its delete, as the task kind is asked
// anything, and read, which is reading what it wrote. It is changed as a VM
// is (vmKind.UpdatePayload): its name, which is its task's record's, and its
// ports, network and resources, which are its task's ports, network policy
// (network.PolicyOf) and limits, held to what one VM may be given at most and
// to its node's room. Its task is asked for its update (taskKind.UpdatePayload),
// and its node then gives its run what it was changed to, by booting its VM
// afresh, which runs its snippet again from the start, as a VM somebody
// changed reboots. Its lifetime is its snippet's own timeout as much as its
// task's ttl, so a change to it is refused as CodeRefused, under the lifetime
// it is asked with; a start, a restart and a restore are refused so too,
// under vm: a run is the code runner's to start, and it is gone once it has
// ended. Its terminal is its task's, a stream anybody opens on the ingress
// (GET /tasks/{uuid}/attach), and its snapshots are the snapshot kind's,
// taken of it as of a VM, by whoever asks for one: neither is asked of it
// here.
package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	controlPlaneVMs "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/quota"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// CodeRefused is what a run is refused with when it is asked to be
	// started, restarted or restored, or kept for longer than its snippet
	// runs, which only a VM somebody asked for can be.
	CodeRefused = "managed_by_code_runner"

	// tries is how many times a run is read and asked again when something
	// else wrote its task in the meantime.
	tries = 3
)

// Room says whether the node holding a VM has room for it to be given
// resources instead of what it has now (placement.Placement).
type Room interface {
	Fits(ctx context.Context, v vmKind.VM, resources vmKind.Resources) (bool, error)
}

// Runs are the code runner's runs, read from its tasks.
type Runs struct {
	resources resource.Repository

	// registry is where the task kind is, whose commands a run is asked as
	// any task is, through dispatcher.
	registry   *kind.Registry[kind.ControlPlaneBinding]
	dispatcher *dispatch.Dispatcher

	// limits are what one VM may be given, which a run changed is held to at
	// most, and room what its node has room for.
	limits quota.Limits
	room   Room

	logger *slog.Logger
}

var _ kind.Extras = &Runs{}

// New is the code runner's runs, read from the tasks resources keeps and
// asked for their commands through dispatcher, as the task kind in registry
// says, a run changed held to the most limits let one VM be given and to what
// room says its node has room for, and what could not be asked of one once it
// was changed told to logger.
func New(resources resource.Repository, registry *kind.Registry[kind.ControlPlaneBinding], dispatcher *dispatch.Dispatcher, limits quota.Limits, room Room, logger *slog.Logger) *Runs {
	return &Runs{resources: resources, registry: registry, dispatcher: dispatcher, limits: limits, room: room, logger: logger}
}

// All is every run there is now, as VMs, newest first.
func (r *Runs) All(ctx context.Context) ([]kind.Raw, error) {
	records, _, err := r.resources.GetAll(ctx, taskKind.Name, resource.Filter{OwnerUUID: task.GuestOwnerUUID}, 0, 0)
	if err != nil {
		return nil, err
	}

	runs := make([]kind.Raw, 0, len(records))
	for _, record := range records {
		run, err := manifest(record)
		if err != nil {
			return nil, err
		}

		runs = append(runs, run)
	}

	slices.SortStableFunc(runs, newestFirst)

	return runs, nil
}

// One is the run uuid names, as the VM it runs in, or domain.ErrNotExists.
func (r *Runs) One(ctx context.Context, uuid string) (kind.Raw, error) {
	record, err := r.record(ctx, uuid)
	if err != nil {
		return kind.Raw{}, err
	}

	return manifest(record)
}

// Act asks a run for what a VM is asked: a stop is its task's stop, a delete
// its task's delete, an update its task's update (update), and anything else
// is refused as CodeRefused.
func (r *Runs) Act(ctx context.Context, run kind.Raw, action string, payload []byte) (kind.Raw, bool, domain.ValidationErrors, error) {
	switch action {
	case vmKind.ActionStop:
		return r.ask(ctx, run.Metadata.UUID, taskKind.ActionStop)
	case vmKind.ActionDelete:
		return r.ask(ctx, run.Metadata.UUID, taskKind.ActionDelete)
	case vmKind.ActionUpdate:
		return r.update(ctx, run.Metadata.UUID, payload)
	}

	if _, err := r.record(ctx, run.Metadata.UUID); err != nil {
		return kind.Raw{}, false, nil, err
	}

	return kind.Raw{}, false, refusal(), nil
}

// ask asks a run's task for one of its commands, and is the run as it left
// it, or gone when there was nothing anywhere to delete.
//
// What it is asked is written down before its node is asked, whatever comes
// of asking, as it is of any task. A stop it cannot get to from where it is,
// one stopping already or not running yet, is refused as
// invalid_state_transition, and the task is to stop all the same once it
// can, unless it is being deleted. A delete is never refused, and one being
// deleted already is left to it.
func (r *Runs) ask(ctx context.Context, uuid string, action string) (kind.Raw, bool, domain.ValidationErrors, error) {
	binding, registered := r.registry.Lookup(taskKind.Name)
	if !registered {
		return kind.Raw{}, false, nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, taskKind.Name)
	}

	d := binding.Descriptor()
	a, _ := d.Action(action)

	for try := 1; ; try++ {
		record, err := r.record(ctx, uuid)
		if err != nil {
			return kind.Raw{}, false, nil, err
		}

		common, err := record.Common()
		if err != nil {
			return kind.Raw{}, false, nil, err
		}

		if action == taskKind.ActionDelete && common.Expected == kind.Deleted && record.Pending != nil && record.Pending.Action == taskKind.ActionDelete {
			after, err := manifest(record)

			return after, false, nil, err
		}

		var (
			asked   dispatch.Asked
			invalid domain.ValidationErrors
		)

		switch {
		case d.Allows(action, common.State):
			asked, invalid, err = r.dispatcher.Ask(ctx, binding, record, action, nil, true)

		// what was asked is what it is expected to be, once it can be, unless
		// it is on its way to being deleted, which nothing comes back from.
		case common.Expected != kind.Deleted:
			_, err = r.dispatcher.Desire(ctx, record, a.Desires, true)
			invalid = domain.ValidationErrors{"action": "invalid_state_transition"}

		default:
			invalid = domain.ValidationErrors{"action": "invalid_state_transition"}
		}

		if errors.Is(err, resource.ErrConflict) && try < tries {
			continue
		} else if err != nil {
			return kind.Raw{}, false, nil, err
		}

		if len(invalid) > 0 {
			return kind.Raw{}, false, domain.ValidationErrors{"vm": "invalid_state_transition"}, nil
		}

		delivered, err := r.dispatcher.Deliver(ctx, asked, 0)
		if err != nil {
			return kind.Raw{}, false, nil, err
		}

		if delivered.Gone {
			return kind.Raw{}, true, nil, nil
		}

		after, err := manifest(resource.Record{Raw: delivered.Resource})

		return after, false, nil, err
	}
}

// update changes a run as a VM is changed: what a VM's update asks of it,
// as its task's update says it (changed), asked of its task as the task kind
// is asked any of its commands, and followed at once by what the task kind
// asks of its task as it now is, its reconfigure when its ports, network or
// resources were changed, which its node then carries out. It is the run as
// that left it: restarting, while its node gives its run what it was changed
// to. What is refused is said under the fields a VM's update is asked with.
//
// Only a run that runs is changed: one on its way anywhere, restarting among
// it, is refused as invalid_state_transition, and nothing changes.
func (r *Runs) update(ctx context.Context, uuid string, payload []byte) (kind.Raw, bool, domain.ValidationErrors, error) {
	var asked vmKind.UpdatePayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &asked); err != nil {
			return kind.Raw{}, false, domain.ValidationErrors{"payload": "invalid_value"}, nil
		}
	}

	if invalid := asked.Validate(); len(invalid) > 0 {
		return kind.Raw{}, false, invalid, nil
	}

	binding, registered := r.registry.Lookup(taskKind.Name)
	if !registered {
		return kind.Raw{}, false, nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, taskKind.Name)
	}

	for try := 1; ; try++ {
		record, err := r.record(ctx, uuid)
		if err != nil {
			return kind.Raw{}, false, nil, err
		}

		t, err := kind.Decode[taskKind.Spec, taskKind.Status](record.Raw)
		if err != nil {
			return kind.Raw{}, false, nil, err
		}

		update, refused, err := r.changed(ctx, t, asked)
		if err != nil || len(refused) > 0 {
			return kind.Raw{}, false, refused, err
		}

		changes, err := json.Marshal(update)
		if err != nil {
			return kind.Raw{}, false, nil, err
		}

		applied, invalid, err := r.dispatcher.Ask(ctx, binding, record, taskKind.ActionUpdate, changes, true)
		if errors.Is(err, resource.ErrConflict) && try < tries {
			continue
		} else if err != nil {
			return kind.Raw{}, false, nil, err
		}

		if len(invalid) > 0 {
			return kind.Raw{}, false, asVM(invalid), nil
		}

		// what could not be asked is the reconcile loop's to ask again, and
		// no reason to answer the change it followed with a failure.
		followed, err := r.dispatcher.Follow(ctx, binding, applied.Record)
		if err != nil {
			r.logger.ErrorContext(ctx, "could not ask a run just changed for what its task kind asks", "error", err, "uuid", uuid)
		}

		delivered, err := r.dispatcher.Deliver(ctx, followed, 0)
		if err != nil {
			return kind.Raw{}, false, nil, err
		}

		after, err := manifest(resource.Record{Raw: delivered.Resource})

		return after, false, nil, err
	}
}

// changed is the update of its task a VM's update of a run is, or why it
// cannot be, under the field of the VM's update that asked for it.
//
// Its name is its task's, and its ports are its task's, as they are. Its
// network is its task's policy, the one the network asked for maps to,
// either way that names nothing staying as it is. Its resources are its
// task's limits: cores where its vCPUs change, and otherwise the cores it
// had, of which its vCPUs are the whole number up; memory and disk as they
// are. A resource it changes is held to what one VM may be given at most, and
// all of them to its node's room; one it leaves as it was is held to
// nothing, and neither is any to the least a VM may be given, which the code
// runner gives its runs less than (a hundred MiB of disk to a VM's GiB), and
// which the task kind holds them to its own least of (task.MinMemory). Its
// lifetime is its snippet's, and a change of it is refused.
func (r *Runs) changed(ctx context.Context, t taskKind.Task, asked vmKind.UpdatePayload) (taskKind.UpdatePayload, domain.ValidationErrors, error) {
	if asked.Lifetime != nil && *asked.Lifetime != t.Spec.TTL {
		return taskKind.UpdatePayload{}, domain.ValidationErrors{"lifetime_seconds": CodeRefused}, nil
	}

	update := taskKind.UpdatePayload{Name: asked.Name, Ports: asked.Ports}

	if asked.Network != nil {
		wanted := t.Spec.Policy().VMNetwork()

		if len(asked.Network.Ingress) > 0 {
			wanted.Ingress = asked.Network.Ingress
		}

		if len(asked.Network.Egress) > 0 {
			wanted.Egress = asked.Network.Egress
		}

		if policy := network.PolicyOf(wanted); policy != t.Spec.Policy() {
			update.NetworkPolicy = &policy
		}
	}

	if asked.Resources == nil {
		return update, nil, nil
	}

	had := vmKind.ResourcesOf(t.Spec.Limits.ResourceLimits().VMResources())
	wanted := *asked.Resources

	if refused := r.bounded(had, wanted); len(refused) > 0 {
		return taskKind.UpdatePayload{}, refused, nil
	}

	limits := t.Spec.Limits

	if wanted.CPUs != had.CPUs {
		limits.CPU = float64(wanted.CPUs)
	}

	limits.Memory, limits.Disk = wanted.Memory, wanted.Disk

	if limits == t.Spec.Limits {
		return update, nil, nil
	}

	fits, err := r.room.Fits(ctx, Manifest(t), wanted)
	if err != nil {
		return taskKind.UpdatePayload{}, nil, err
	}

	if !fits {
		return taskKind.UpdatePayload{}, domain.ValidationErrors{"resources": controlPlaneVMs.ReasonNoCapacity}, nil
	}

	update.Limits = &limits

	return update, nil, nil
}

// bounded is what is wrong with the resources a run would be given in place
// of what it had: each it changes past what one VM may be given, or to
// nothing. What one VM may be given at least is not a run's to be held to.
func (r *Runs) bounded(had vmKind.Resources, wanted vmKind.Resources) domain.ValidationErrors {
	refused := r.limits.Bounds("", vm.KindMachine, wanted)

	for field, changes := range map[string]bool{
		"resources.cpus":   wanted.CPUs != had.CPUs,
		"resources.memory": wanted.Memory != had.Memory,
		"resources.disk":   wanted.Disk != had.Disk,
	} {
		if !changes || refused[field] == quota.CodeTooSmall {
			delete(refused, field)
		}
	}

	return refused
}

// asVM is what a run's task was refused, under the fields of the VM's update
// that asked for them: its limits are its resources, its network policy its
// network, and a command it may not be asked where it is is the VM's.
func asVM(invalid domain.ValidationErrors) domain.ValidationErrors {
	fields := map[string]string{
		"limits.cpu":     "resources.cpus",
		"limits.memory":  "resources.memory",
		"limits.disk":    "resources.disk",
		"network_policy": "network",
		"action":         "vm",
	}

	said := make(domain.ValidationErrors, len(invalid))
	for field, code := range invalid {
		if vmField, renamed := fields[field]; renamed {
			field = vmField
		}

		said[field] = code
	}

	return said
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

	record, err := r.record(ctx, run.Metadata.UUID)
	if err != nil {
		return nil, nil, err
	}

	t, err := kind.Decode[taskKind.Spec, taskKind.Status](record.Raw)
	if err != nil {
		return nil, nil, err
	}

	answer, err := json.Marshal(Logs(t, options))

	return answer, nil, err
}

// record is the task of the run uuid names.
func (r *Runs) record(ctx context.Context, uuid string) (resource.Record, error) {
	if len(uuid) == 0 {
		return resource.Record{}, domain.ErrNotExists
	}

	return r.resources.GetOneByOwner(ctx, taskKind.Name, task.GuestOwnerUUID, uuid)
}

// refusal is what a run is refused a start, a restart and a restore with,
// and anything else nobody asks of a VM.
func refusal() domain.ValidationErrors {
	return domain.ValidationErrors{"vm": CodeRefused}
}

// Logs is what a run has written, as the lines of a VM's log, and whether
// there were more of them than an answer carries.
//
// A run is a job, and a job's output rides every heartbeat its node sends of
// its task and is kept on the task. Its lines have no time to them, so each
// is given the moment the run was made, a nanosecond after the line before
// it: that keeps them in the order they were written, keeps two that say the
// same thing apart, and lets whoever is following the log ask for what came
// after the last line they read, as they would of a VM's. Since leaves out
// the lines before it, and the last Tail are kept, or as many as an answer
// carries.
func Logs(t taskKind.Task, options vmKind.LogsPayload) vmKind.Logs {
	lines := make([]vmKind.LogLine, 0)

	var output string
	if t.Status.Run != nil {
		output = strings.TrimSuffix(t.Status.Run.Output, "\n")
	}

	if len(output) == 0 {
		return vmKind.Logs{Lines: lines}
	}

	for i, line := range strings.Split(output, "\n") {
		at := t.Metadata.CreatedAt.Add(time.Duration(i))
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
// It is its task's uuid, name and slug, and the guest's: a machine booted from
// the runner's image, given what its task was limited to the way a node gives
// it, with the network its task's policy maps to and a disk thrown away with
// it. Its lifetime is its task's ttl, counted from when its run started, and
// its state is its task's, in a VM's words. Its node reports no stats for a
// run, so it has none to show.
func Manifest(t taskKind.Task) vmKind.VM {
	var startedAt time.Time
	if t.Status.Run != nil {
		startedAt = t.Status.Run.StartedAt
	}

	// when it ended, for one that has: when it came to rest where it is.
	var endedAt time.Time
	if taskKind.Ended(t.Status.State) {
		endedAt = t.Status.Since
	}

	return vmKind.VM{
		Kind: vmKind.Name,
		Metadata: kind.Metadata{
			UUID:      t.Metadata.UUID,
			Name:      t.Metadata.Name,
			Slug:      t.Metadata.Slug,
			OwnerUUID: t.Metadata.OwnerUUID,
			Labels: map[string]string{
				vmKind.LabelManagedBy: vmKind.ManagedByCodeRunner,
			},
			Node:      t.Metadata.Node,
			Lifetime:  t.Spec.TTL,
			ExpiresAt: expiresAt(t),
			CreatedAt: t.Metadata.CreatedAt,
			UpdatedAt: latest(t.Metadata.CreatedAt, startedAt, endedAt),
		},
		Spec: vmKind.Spec{
			Image:     t.Spec.Image,
			Resources: vmKind.ResourcesOf(t.Spec.Limits.ResourceLimits().VMResources()),
			Ports:     vmKind.Normalized(t.Spec.Ports),
			Network:   networkOf(t.Spec.Policy().VMNetwork()),
		},
		Status: vmKind.Status{
			Status: kind.Status{
				State:      stateOf(t.Status.State),
				Expected:   stateOf(t.Status.Expected),
				Reason:     t.Status.Reason,
				ObservedAt: t.Status.ObservedAt,
			},
			StartedAt: startedAt,
		},
	}
}

// manifest is a task's record as the VM its run is.
func manifest(record resource.Record) (kind.Raw, error) {
	t, err := kind.Decode[taskKind.Spec, taskKind.Status](record.Raw)
	if err != nil {
		return kind.Raw{}, err
	}

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
func stateOf(state kind.State) kind.State {
	switch state {
	case taskKind.Created:
		return vmKind.Created
	case taskKind.Scheduled:
		return vmKind.Scheduled
	case taskKind.Running:
		return vmKind.Running
	case taskKind.Stopping:
		return vmKind.Stopping
	case taskKind.Stopped, taskKind.Completed:
		return vmKind.Stopped
	case taskKind.Failed:
		return vmKind.Failed
	case taskKind.Restarting:
		return vmKind.Restarting
	case taskKind.Deleting:
		return vmKind.Deleting
	case taskKind.Deleted:
		return vmKind.Deleted
	default:
		return ""
	}
}

// expiresAt is when a run is stopped for having run long enough: the
// deadline its node set as it came up, which is its ttl counted from when it
// started. One that has not come up yet has none, and neither has one that
// may run for as long as it likes.
func expiresAt(t taskKind.Task) time.Time {
	run := t.Status.Run
	if run == nil {
		return time.Time{}
	}

	if !run.Deadline.IsZero() {
		return run.Deadline
	}

	if t.Spec.TTL > 0 && !run.StartedAt.IsZero() {
		return run.StartedAt.Add(t.Spec.TTL)
	}

	return time.Time{}
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
