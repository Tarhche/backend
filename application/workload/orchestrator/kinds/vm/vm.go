// Package vm is the vm kind's node strategy: what a node does to the VMs it
// holds, and what it says they are doing.
//
// Everything is done through the node's engine (vm.Engine), which knows
// instances rather than records: a VM is the instance named by its uuid,
// labelled with whose it is, the slug its ports are served under, that it is
// a VM, and that it is a Docker VM when it is one (vmKind.LabelDocker). What
// the node knows of a VM is what a command carries, the VM as the control
// plane recorded it, and what the engine says.
//
// A command is carried out the way it always was:
//
//   - create and start make a VM run: the instance the engine holds is
//     booted, and one it holds none of is made, from its image or from its
//     source's snapshot, streamed from where snapshots are stored. That is
//     also how a VM its node lost is made again;
//   - stop, restart, reconfigure and restore do what they say to the
//     instance, a restore streaming the snapshot's archive into the engine,
//     which replaces the instance's disk with it;
//   - delete removes the instance, disk and all.
//
// Whatever is open into a VM, a Docker VM's dockerd connections, is let go
// of when it goes down. What came of a command is what the instance is doing
// afterwards, with the config it was given when it was made, reconfigured or
// restored, which is how the control plane knows a change of its ports
// reached it.
//
// Its state is every VM instance the engine holds, with a sample of what each
// running one uses, its CPU 0 to 100 of all the vCPUs it was given. While a
// command is carried out on a VM, the VM is said to be still in flight, as
// the command left it on the control plane, so that what it is doing halfway
// through is not taken for what the command came to. Its terminal is a shell
// inside it, for its owner alone, and its ports are wherever its engine
// published them.
package vm

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/internal/reply"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// sampleEvery is how often a running VM is sampled: every other
	// heartbeat, as often as VMs were before they were a kind.
	sampleEvery = 2 * time.Second

	// sampleKept is how long a sample is shown when a newer one could not be
	// taken in time, rather than nothing.
	sampleKept = 10 * time.Second

	// sampling is how many VMs are sampled at once.
	sampling = 8
)

// shell is bash where the image has it and sh where it does not. It is asked
// of sh, which every image with a shell has, rather than tried and fallen back
// from, so a VM without bash costs nothing more than one that has it.
var shell = []string{"/bin/sh", "-c", "if [ -x /bin/bash ]; then exec /bin/bash; fi; exec /bin/sh"}

// Forgetter lets go of what this node keeps open into a VM: a client of a
// Docker VM's dockerd, whose connections are commands running inside the VM
// and so do not outlive it going down.
type Forgetter interface {
	Forget(vmUUID string)
}

// Gauges is where what this node holds is published for the dashboards.
type Gauges interface {
	// Node records what the node offers, what it has given, and how many VMs
	// are in each state.
	Node(ctx context.Context, node string, info vm.Info, counts map[vm.State]int)

	// VMHost records whether this node's last call to its engine succeeded.
	VMHost(ctx context.Context, node string, up bool)
}

// Node is the vm kind's node strategy.
type Node struct {
	engine      vm.Engine
	archives    snapshotKind.Store
	connections Forgetter
	gauges      Gauges
	nodeName    string
	now         func() time.Time

	lock sync.Mutex

	// underway are the VMs a command is being carried out on, by uuid, and
	// the state the command left each in on the control plane.
	underway map[string]kind.State

	// samples are the last sample of each running VM.
	samples map[string]*vmKind.Stats
}

var (
	_ kind.Node[vmKind.Spec, vmKind.Status] = &Node{}
	_ kind.Attacher                         = &Node{}
	_ kind.Exposer                          = &Node{}
)

// New is the strategy that runs VMs on engine, restores them from what
// archives keeps, lets go of what connections keeps open into them, and
// publishes what nodeName holds to gauges.
func New(engine vm.Engine, archives snapshotKind.Store, connections Forgetter, gauges Gauges, nodeName string) *Node {
	return &Node{
		engine:      engine,
		archives:    archives,
		connections: connections,
		gauges:      gauges,
		nodeName:    nodeName,
		now:         time.Now,
		underway:    make(map[string]kind.State),
		samples:     make(map[string]*vmKind.Stats),
	}
}

// Execute carries out one of a VM's commands, and is what it left the VM
// as: what its instance is doing afterwards.
func (n *Node) Execute(ctx context.Context, v vmKind.VM, action string, payload any) (kind.Outcome[vmKind.Status], error) {
	n.begin(v)
	defer n.end(v.Metadata.UUID)

	var (
		status vmKind.Status
		err    error
	)

	switch action {
	case vmKind.ActionCreate, vmKind.ActionStart:
		status, err = n.up(ctx, v)
	case vmKind.ActionStop:
		status, err = n.stop(ctx, v)
	case vmKind.ActionRestart:
		status, err = n.restart(ctx, v)
	case vmKind.ActionReconfigure:
		status, err = n.reconfigure(ctx, v)
	case vmKind.ActionRestore:
		restore, _ := payload.(vmKind.RestorePayload)
		status, err = n.restore(ctx, v, restore.SnapshotUUID)
	case vmKind.ActionDelete:
		err = n.delete(ctx, v)
	default:
		err = fmt.Errorf("%w: a vm cannot be %s on its node", kind.ErrUnknownAction, action)
	}

	if err != nil {
		return kind.Outcome[vmKind.Status]{}, err
	}

	return kind.Outcome[vmKind.Status]{Status: status}, nil
}

// up makes a VM run: the instance the engine holds is booted, unless it runs
// already, and one it holds none of is made, from its image or from its
// source's snapshot. A VM asked for again, by a command delivered again or by
// the control plane bringing it back, is what was asked for, and is left as
// it is.
func (n *Node) up(ctx context.Context, v vmKind.VM) (vmKind.Status, error) {
	held, err := n.engine.Inspect(ctx, v.Metadata.UUID)

	switch {
	case err == nil:
		if held.State != vm.InstanceRunning {
			if err := n.engine.Start(ctx, v.Metadata.UUID); err != nil {
				return vmKind.Status{}, err
			}
		}

		return n.inspected(ctx, v, false)
	case !errors.Is(err, domain.ErrNotExists):
		return vmKind.Status{}, err
	}

	spec := Spec(v)

	if v.Spec.Source != nil && len(v.Spec.Source.Snapshot) > 0 {
		if err := n.restored(ctx, spec, v.Spec.Source.Snapshot); err != nil {
			return vmKind.Status{}, err
		}
	} else if _, err := n.engine.Create(ctx, spec); err != nil {
		return vmKind.Status{}, err
	}

	return n.inspected(ctx, v, true)
}

// stop stops a VM, keeping its disk. One that is not running is what was
// asked for already, and one this node holds nothing of is not running.
func (n *Node) stop(ctx context.Context, v vmKind.VM) (vmKind.Status, error) {
	// once it is down, nothing opened into it before still leads anywhere.
	defer n.connections.Forget(v.Metadata.UUID)

	held, err := n.engine.Inspect(ctx, v.Metadata.UUID)
	if err != nil {
		return n.missing(v, err)
	}

	if held.State == vm.InstanceRunning || held.State == vm.InstanceCreated {
		if err := n.engine.Stop(ctx, v.Metadata.UUID); err != nil {
			return vmKind.Status{}, err
		}
	}

	return n.inspected(ctx, v, false)
}

// restart stops a running VM and boots it again in place, and boots one that
// is not running, which is what somebody who asked for a restart wants to end
// with. One this node holds nothing of is made.
func (n *Node) restart(ctx context.Context, v vmKind.VM) (vmKind.Status, error) {
	held, err := n.engine.Inspect(ctx, v.Metadata.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return n.up(ctx, v)
	} else if err != nil {
		return vmKind.Status{}, err
	}

	defer n.connections.Forget(v.Metadata.UUID)

	if held.State != vm.InstanceRunning {
		err = n.engine.Start(ctx, v.Metadata.UUID)
	} else {
		err = n.engine.Restart(ctx, v.Metadata.UUID)
	}

	if err != nil {
		return vmKind.Status{}, err
	}

	return n.inspected(ctx, v, false)
}

// reconfigure gives a VM's instance the ports, network and resources the VM
// now has, which restarts it when the engine has to. One this node holds
// nothing of is made with them when it is next made, which is as good as
// given them now.
func (n *Node) reconfigure(ctx context.Context, v vmKind.VM) (vmKind.Status, error) {
	defer n.connections.Forget(v.Metadata.UUID)

	if _, err := n.engine.Reconfigure(ctx, Spec(v)); errors.Is(err, domain.ErrNotExists) {
		status, err := n.missing(v, err)
		config := v.Spec.Config()
		status.Applied = &config

		return status, err
	} else if err != nil {
		return vmKind.Status{}, err
	}

	return n.inspected(ctx, v, true)
}

// restore replaces a VM's disk with what a snapshot holds: the VM is stopped,
// the snapshot's archive is streamed from where snapshots are kept into the
// engine, and the engine replaces the instance under the same name, booted.
// The VM keeps its uuid, its slug and its ports. One that is not here is
// restored all the same: the engine makes it.
func (n *Node) restore(ctx context.Context, v vmKind.VM, snapshotUUID string) (vmKind.Status, error) {
	// whatever was open into the disk being replaced leads nowhere now.
	defer n.connections.Forget(v.Metadata.UUID)

	if err := n.engine.Stop(ctx, v.Metadata.UUID); err != nil && !errors.Is(err, domain.ErrNotExists) {
		return vmKind.Status{}, err
	}

	if err := n.restored(ctx, Spec(v), snapshotUUID); err != nil {
		return vmKind.Status{}, err
	}

	status, err := n.inspected(ctx, v, true)
	if err != nil {
		return vmKind.Status{}, err
	}

	status.RestoredAt = n.now()

	return status, nil
}

// delete removes a VM, disk and all. One that was not here is gone already.
// Its snapshots are not the node's to remove: they outlive it.
func (n *Node) delete(ctx context.Context, v vmKind.VM) error {
	defer n.connections.Forget(v.Metadata.UUID)

	return n.engine.Delete(ctx, v.Metadata.UUID)
}

// restored gives the instance spec names the disk a snapshot holds: the
// archive is read from where snapshots are stored and streamed into the
// engine as it arrives, so it is never held whole, for as long as a disk is
// given to stream (snapshotKind.TransferTimeout), as a snapshot taken is.
func (n *Node) restored(ctx context.Context, spec vm.Spec, snapshotUUID string) error {
	ctx, cancel := context.WithTimeout(ctx, snapshotKind.TransferTimeout)
	defer cancel()

	archive, err := n.archives.Read(ctx, snapshotKind.ObjectKey(snapshotUUID))
	if err != nil {
		return fmt.Errorf("the snapshot cannot be read: %w", err)
	}
	defer archive.Close()

	_, err = n.engine.Restore(ctx, spec, archive)

	return err
}

// inspected is what a VM's instance is doing after a command, with the
// config the command gave it when it gave it one.
func (n *Node) inspected(ctx context.Context, v vmKind.VM, applied bool) (vmKind.Status, error) {
	held, err := n.engine.Inspect(ctx, v.Metadata.UUID)
	if err != nil {
		return n.missing(v, err)
	}

	status := statusOf(held)

	if applied {
		config := v.Spec.Config()
		status.Applied = &config
	}

	return status, nil
}

// missing is what a command left a VM this node holds nothing of as: not
// running, which ends a stop and is what has a VM expected running made
// again. A failed one is left saying why it failed, which nothing here
// knows better: nothing is said of it.
func (n *Node) missing(v vmKind.VM, err error) (vmKind.Status, error) {
	if !errors.Is(err, domain.ErrNotExists) {
		return vmKind.Status{}, err
	}

	if v.Status.State == vmKind.Failed {
		return vmKind.Status{}, nil
	}

	return vmKind.Status{Status: kind.Status{State: vmKind.Stopped}}, nil
}

// Query answers a VM's log and a sample of what it uses, read from the
// engine as they are now. A VM this node holds nothing of is not running.
func (n *Node) Query(ctx context.Context, v vmKind.VM, action string, payload any) (any, error) {
	switch action {
	case vmKind.ActionLogs:
		asked, _ := payload.(vmKind.LogsPayload)

		return n.logs(ctx, v.Metadata.UUID, asked)
	case vmKind.ActionStats:
		stats, err := n.engine.Stats(ctx, v.Metadata.UUID)
		if errors.Is(err, domain.ErrNotExists) {
			return nil, fmt.Errorf("%w: this node holds nothing of %q", vm.ErrNotRunning, v.Metadata.UUID)
		} else if err != nil {
			return nil, err
		}

		return vmKind.StatsOf(stats), nil
	}

	return nil, fmt.Errorf("%w: a vm has no %q", kind.ErrUnknownAction, action)
}

// logs reads a VM's log, which nothing stores: it is asked of the engine
// whenever somebody wants to read it. An answer has to fit in one NATS
// message, so a log is read from its end and cut at the most lines an answer
// carries, and at what fits; one cut short says so.
func (n *Node) logs(ctx context.Context, uuid string, asked vmKind.LogsPayload) (vmKind.Logs, error) {
	// one line more than an answer carries is how a log that does not fit is
	// told from one that just does.
	tail := asked.Tail
	if tail == 0 || tail > noderequest.MaxLogLines {
		tail = noderequest.MaxLogLines + 1
	}

	read, err := n.engine.Logs(ctx, uuid, vm.LogOptions{Since: asked.Since, Tail: tail})
	if errors.Is(err, domain.ErrNotExists) {
		return vmKind.Logs{}, fmt.Errorf("%w: this node holds nothing of %q", vm.ErrNotRunning, uuid)
	} else if err != nil {
		return vmKind.Logs{}, err
	}

	truncated := false
	if len(read) > noderequest.MaxLogLines {
		read, truncated = read[len(read)-noderequest.MaxLogLines:], true
	}

	lines := make([]vmKind.LogLine, len(read))
	for i, line := range read {
		lines[i] = vmKind.LogLineOf(line)
	}

	fitted, cut := reply.Last(lines)

	return vmKind.Logs{Lines: fitted, Truncated: truncated || cut}, nil
}

// State is every VM this node's engine holds, with a sample of what each
// running one uses. Whether the engine answered, and what it offers and has
// given, are published for the dashboards as well.
func (n *Node) State(ctx context.Context) (kind.Report[vmKind.Status], error) {
	info, err := n.engine.Info(ctx)
	if err != nil {
		n.gauges.VMHost(ctx, n.nodeName, false)

		return kind.Report[vmKind.Status]{}, err
	}

	instances, err := n.engine.List(ctx)
	if err != nil {
		n.gauges.VMHost(ctx, n.nodeName, false)

		return kind.Report[vmKind.Status]{}, err
	}

	n.gauges.VMHost(ctx, n.nodeName, true)

	var held []vm.Instance

	for _, instance := range instances {
		if instance.Labels[vm.LabelPurpose] == vm.PurposeVM {
			held = append(held, instance)
		}
	}

	samples := n.sample(ctx, held)

	report := kind.Report[vmKind.Status]{Instances: make([]kind.Observed[vmKind.Status], 0, len(held))}
	counts := make(map[vm.State]int)

	n.lock.Lock()
	for _, instance := range held {
		uuid := UUIDOf(instance)
		status := statusOf(instance)
		status.Stats = samples[uuid]

		if state, busy := n.underway[uuid]; busy {
			status.State = state
		}

		report.Instances = append(report.Instances, kind.Observed[vmKind.Status]{UUID: uuid, Status: status})
		counts[countedAs(instance.State)]++
	}
	n.lock.Unlock()

	n.gauges.Node(ctx, n.nodeName, info, counts)

	slices.SortFunc(report.Instances, func(a, b kind.Observed[vmKind.Status]) int {
		switch {
		case a.UUID < b.UUID:
			return -1
		case a.UUID > b.UUID:
			return 1
		}

		return 0
	})

	return report, nil
}

// sample is a sample of what each running instance uses, by VM, taken at
// most every other heartbeat and a few at once, and given up on when the
// heartbeat cannot wait for it any longer: the last one is kept a while
// rather than shown as nothing. Only a running VM uses anything worth
// sampling.
func (n *Node) sample(ctx context.Context, held []vm.Instance) map[string]*vmKind.Stats {
	now := n.now()
	samples := make(map[string]*vmKind.Stats, len(held))

	type sampled struct {
		uuid  string
		stats *vmKind.Stats
	}

	var due []vm.Instance

	n.lock.Lock()
	for _, instance := range held {
		uuid := UUIDOf(instance)
		last, kept := n.samples[uuid]

		switch {
		case instance.State != vm.InstanceRunning:
			delete(n.samples, uuid)
		case kept && now.Sub(last.SampledAt) < sampleEvery:
			samples[uuid] = last
		default:
			due = append(due, instance)
		}
	}

	// what is no longer held is no longer sampled.
	maps.DeleteFunc(n.samples, func(uuid string, _ *vmKind.Stats) bool {
		return !slices.ContainsFunc(held, func(instance vm.Instance) bool { return UUIDOf(instance) == uuid })
	})
	n.lock.Unlock()

	answers := make(chan sampled, len(due))
	slots := make(chan struct{}, sampling)

	for _, instance := range due {
		go func() {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				answers <- sampled{uuid: UUIDOf(instance)}

				return
			}
			defer func() { <-slots }()

			stats, err := n.engine.Stats(ctx, instance.ID)
			if err != nil {
				// sampled again on the next beat; a VM is no less there for
				// one sample missing.
				answers <- sampled{uuid: UUIDOf(instance)}

				return
			}

			if stats.SampledAt.IsZero() {
				stats.SampledAt = n.now()
			}

			answers <- sampled{uuid: UUIDOf(instance), stats: vmKind.StatsOf(stats)}
		}()
	}

	taken := make(map[string]*vmKind.Stats, len(due))

collecting:
	for range due {
		select {
		case answer := <-answers:
			taken[answer.uuid] = answer.stats
		case <-ctx.Done():
			// the beat goes without what is still being sampled, and the
			// last sample of it is shown meanwhile.
			break collecting
		}
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	for _, instance := range due {
		uuid := UUIDOf(instance)

		if stats := taken[uuid]; stats != nil {
			n.samples[uuid] = stats
			samples[uuid] = stats

			continue
		}

		samples[uuid] = n.kept(uuid, now)
	}

	return samples
}

// kept is the last sample of a VM, while it is recent enough to show. It is
// called with the lock held.
func (n *Node) kept(uuid string, now time.Time) *vmKind.Stats {
	last, kept := n.samples[uuid]
	if !kept || now.Sub(last.SampledAt) > sampleKept {
		return nil
	}

	return last
}

// Endpoint is where port p of the VM a slug names is published on this
// node, or its lowest port when p is none. Its engine publishes the ports a
// VM was given and nothing else, and nothing at all of one whose ingress is
// denied, so a port that is not there is not exposed, whatever is asked for.
func (n *Node) Endpoint(ctx context.Context, slug string, p port.Port) (kind.Endpoint, error) {
	instances, err := n.engine.List(ctx)
	if err != nil {
		return kind.Endpoint{}, err
	}

	held, found := bySlug(instances, slug)
	if !found {
		return kind.Endpoint{}, fmt.Errorf("%w: this node holds no vm %q", domain.ErrNotExists, slug)
	}

	if held.State != vm.InstanceRunning {
		return kind.Endpoint{}, fmt.Errorf("%w: the vm is not running", kind.ErrUnreachable)
	}

	endpoint, found := pick(held.Endpoints, p)
	if !found {
		return kind.Endpoint{}, fmt.Errorf("%w: the vm %q does not expose that port", domain.ErrNotExists, slug)
	}

	return kind.Endpoint{Port: endpoint.Port, Address: endpoint.Address}, nil
}

// Attach opens a terminal in a VM, for its owner alone.
//
// Whose a VM is comes off the VM itself: the control plane labelled it with
// its owner when it asked for it, so the node answers without a database and
// without taking anybody's word for it. A VM always has an owner, so one that
// does not say who is opened for nobody, and neither is anything that is not
// a VM. Somebody who may not open it is told it is not there, so knowing a
// uuid says nothing about whether one exists.
func (n *Node) Attach(ctx context.Context, action string, uuid string, owner string) (kind.Session, error) {
	if action != vmKind.ActionAttach {
		return nil, fmt.Errorf("%w: a vm has no stream %q", kind.ErrUnknownAction, action)
	}

	notTheirs := fmt.Errorf("%w: no vm %q of theirs", domain.ErrNotExists, uuid)

	held, err := n.engine.Inspect(ctx, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, notTheirs
	} else if err != nil {
		return nil, err
	}

	labelled := held.Labels[vm.LabelOwner]
	if held.Labels[vm.LabelPurpose] != vm.PurposeVM || len(labelled) == 0 || labelled != owner {
		return nil, notTheirs
	}

	if held.State != vm.InstanceRunning {
		return nil, fmt.Errorf("%w: the vm is not running", kind.ErrUnreachable)
	}

	// the shell outlives this call: it ends when the terminal is closed, not
	// when the request that opened it is done being made.
	return n.engine.Exec(context.WithoutCancel(ctx), held.ID, vm.ExecOptions{Command: shell, TTY: true})
}

// begin says a command is being carried out on a VM, which is in flight
// while it is, in the state the command left it in on the control plane. One
// whose command leaves it at rest is said to be whatever it is doing.
func (n *Node) begin(v vmKind.VM) {
	if !vmKind.Machine().IsInFlight(v.Status.State) {
		return
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	n.underway[v.Metadata.UUID] = v.Status.State
}

// end says the command being carried out on a VM is done.
func (n *Node) end(uuid string) {
	n.lock.Lock()
	defer n.lock.Unlock()

	delete(n.underway, uuid)
}

// Spec is the spec a VM's instance is made, reconfigured or restored with.
//
// It is named by the VM's uuid, which is what the engine calls it from then
// on, and labelled with whose it is, the slug its ports are served under,
// that it is a VM, which is what puts it in the node's heartbeats, and that
// it is a Docker VM when it is one, which is how the node tells the VMs whose
// dockerds it reads from the rest.
func Spec(v vmKind.VM) vm.Spec {
	labels := map[string]string{
		vm.LabelOwner:   v.Metadata.OwnerUUID,
		vm.LabelVM:      v.Metadata.UUID,
		vm.LabelSlug:    v.Metadata.Slug,
		vm.LabelPurpose: vm.PurposeVM,
	}

	if vmKind.DockerVM(v) {
		labels[vmKind.LabelDocker] = "true"
	}

	return vm.Spec{
		ID:             v.Metadata.UUID,
		Kind:           v.Spec.Flavor,
		Image:          v.Spec.Image,
		Resources:      v.Spec.Resources.VM(),
		Ports:          slices.Clone(v.Spec.Ports),
		Network:        v.Spec.Network.VM(),
		PersistentDisk: v.Spec.PersistentDisk,
		Labels:         labels,
	}
}

// UUIDOf is which VM an instance is: its label says, and a VM is made under
// its own uuid when a label has gone missing.
func UUIDOf(instance vm.Instance) string {
	if uuid := instance.Labels[vm.LabelVM]; len(uuid) > 0 {
		return uuid
	}

	return instance.ID
}

// statusOf is what an instance says a VM is doing, in the kind's words. An
// instance that exists and has not booted yet says nothing either way, and
// one whose main process ended is not running.
func statusOf(instance vm.Instance) vmKind.Status {
	status := vmKind.Status{Endpoints: vmKind.EndpointsOf(instance.Endpoints)}

	switch instance.State {
	case vm.InstanceRunning:
		status.State = vmKind.Running
		status.StartedAt = instance.StartedAt
	case vm.InstanceStopped, vm.InstanceExited:
		status.State = vmKind.Stopped
	case vm.InstanceFailed:
		status.State = vmKind.Failed
		status.Reason = instance.Reason

		if len(status.Reason) == 0 {
			status.Reason = "the vm failed"
		}
	}

	return status
}

// countedAs is what an instance's state counts as among a node's VMs, for
// the dashboards. An instance that exists and has not booted yet is on its
// way up.
func countedAs(state vm.InstanceState) vm.State {
	switch state {
	case vm.InstanceRunning:
		return vm.Running
	case vm.InstanceStopped, vm.InstanceExited:
		return vm.Stopped
	case vm.InstanceCreated:
		return vm.Starting
	default:
		return vm.Failed
	}
}

// bySlug is the VM answering to slug.
func bySlug(instances []vm.Instance, slug string) (vm.Instance, bool) {
	for _, instance := range instances {
		if len(slug) > 0 && instance.Labels[vm.LabelSlug] == slug && instance.Labels[vm.LabelPurpose] == vm.PurposeVM {
			return instance, true
		}
	}

	return vm.Instance{}, false
}

// pick is the endpoint of the port a hostname asked for, or of the lowest
// one exposed when it named none, which is what a VM with a single port needs
// no port in its name for.
func pick(endpoints []vm.Endpoint, requested port.Port) (vm.Endpoint, bool) {
	var (
		picked vm.Endpoint
		found  bool
	)

	for _, endpoint := range endpoints {
		if len(endpoint.Address) == 0 {
			continue
		}

		if requested > 0 {
			if endpoint.Port == requested {
				return endpoint, true
			}

			continue
		}

		if !found || endpoint.Port < picked.Port {
			picked, found = endpoint, true
		}
	}

	return picked, found
}
