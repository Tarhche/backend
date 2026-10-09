// Package blocks is what the node strategies of the building blocks of a
// Docker VM share: the container, image, network and volume kinds', beside it
// in kinds/container, kinds/image, kinds/network and kinds/volume.
//
// What each of them holds on this node is what the dockerds of the node's
// running Docker VMs hold, read whole (Reader): one inventory of each VM, a
// listing of each sort of object, every VM at once and each for no longer
// than the beat allows. One read is shared by the kinds asked in the same
// beat (kind.BeatOf), so a heartbeat costs four requests to each Docker VM
// however many kinds it reports, and none made before the beat is, which
// would say less than the heartbeat claims to: a VM restored a moment before
// says what its restored disk holds. A VM whose dockerd did not answer in
// time is unseen, and says nothing of what is in it; one that is not running
// is not read at all, and what is in it waits on it.
//
// A command reaches the dockerd of the Docker VM its resource lives in
// (Reader.Reach): the VM has to be here, and running or coming up, and its
// dockerd is waited for while it comes up.
package blocks

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// readMargin is how long before a read's deadline the Docker VMs are
	// given up on, so that which of them did not answer is said rather than
	// the read not made at all.
	readMargin = 100 * time.Millisecond

	// readTimeout bounds a read asked for with no deadline of its own.
	readTimeout = 10 * time.Second
)

// ErrGone is a Docker VM this node holds nothing of: whatever lived in it is
// not here either.
var ErrGone = errors.New("its vm is not on this node")

// Daemons are the dockerds of this node's Docker VMs.
type Daemons interface {
	Daemon(vmUUID string) docker.Daemon
}

// Read is what one look into this node's running Docker VMs found.
type Read struct {
	// At is when it was begun: what it found is as of then, or later.
	At time.Time

	// Inventories are what the Docker VMs whose dockerd answered hold, by
	// their uuids.
	Inventories map[string]docker.Inventory

	// Unseen are the running Docker VMs whose dockerd did not answer in time.
	Unseen []string
}

// VMs are the Docker VMs read, in order.
func (r Read) VMs() []string {
	return slices.Sorted(maps.Keys(r.Inventories))
}

// Report is a report of the kind that has nothing in it yet, but which VMs
// were read and which were not: what a kind's state starts from.
func Report[Status any](r Read) kind.Report[Status] {
	return kind.Report[Status]{
		Instances: []kind.Observed[Status]{},
		Read:      r.VMs(),
		Unseen:    slices.Clone(r.Unseen),
	}
}

// Reader looks into the running Docker VMs of this node.
type Reader struct {
	engine  vm.Engine
	daemons Daemons
	now     func() time.Time

	lock sync.Mutex

	// last is the latest begun of the reads made, and reading the latest
	// begun of those being made.
	last    Read
	reading *reading

	// named are the VMs a command named as Docker VMs, which they are
	// whatever their labels say: those made before VMs were labelled.
	named map[string]bool
}

type reading struct {
	begun time.Time
	done  chan struct{}
	read  Read
	err   error
}

// NewReader is a reader that finds the Docker VMs through engine and reads
// them through daemons.
func NewReader(engine vm.Engine, daemons Daemons) *Reader {
	return &Reader{engine: engine, daemons: daemons, now: time.Now, named: make(map[string]bool)}
}

// Read is what this node's running Docker VMs hold now, given until ctx's
// deadline: for a heartbeat, a read begun since the beat was, made already,
// being made, or made now, which the kinds asked in the same beat share; for
// anything else, a read begun since it was asked. An error is that it could
// see nothing: the engine did not say which VMs there are.
func (r *Reader) Read(ctx context.Context) (Read, error) {
	since, beating := kind.BeatOf(ctx)
	if !beating {
		since = r.now()
	}

	r.lock.Lock()

	if !r.last.At.IsZero() && !r.last.At.Before(since) {
		read := r.last
		r.lock.Unlock()

		return read, nil
	}

	current := r.reading
	if current == nil || current.begun.Before(since) {
		current = &reading{begun: r.now(), done: make(chan struct{})}
		r.reading = current

		// made apart from whoever asked first, so that one giving up does
		// not take the read away from the others waiting for it.
		go r.read(ctx, current)
	}

	r.lock.Unlock()

	select {
	case <-current.done:
		return current.read, current.err
	case <-ctx.Done():
		return Read{}, ctx.Err()
	}
}

// read makes a read, bounded by ctx's deadline, and hands it to whoever
// waits for it.
func (r *Reader) read(ctx context.Context, current *reading) {
	bounded, cancel := boundedBy(ctx)
	defer cancel()

	read, err := r.look(bounded, current.begun)

	r.lock.Lock()
	current.read, current.err = read, err

	if err == nil && !read.At.Before(r.last.At) {
		r.last = read
	}

	if r.reading == current {
		r.reading = nil
	}

	r.lock.Unlock()

	close(current.done)
}

// boundedBy is ctx with nothing to cancel it but a deadline: its own, less
// the margin the read gives up early by, or one of its own.
func boundedBy(ctx context.Context) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(ctx)

	if deadline, bounded := ctx.Deadline(); bounded {
		return context.WithDeadline(detached, deadline.Add(-readMargin))
	}

	return context.WithTimeout(detached, readTimeout)
}

// look reads every running Docker VM of this node at once, as of a moment
// it was begun at.
func (r *Reader) look(ctx context.Context, at time.Time) (Read, error) {
	instances, err := r.engine.List(ctx)
	if err != nil {
		return Read{}, err
	}

	var vms []string

	for _, instance := range instances {
		if uuid, docker := r.dockerVM(instance); docker && instance.State == vm.InstanceRunning {
			vms = append(vms, uuid)
		}
	}

	type answer struct {
		vm        string
		inventory docker.Inventory
		err       error
	}

	// buffered for every VM, so one answering after the read gave up on it is
	// not left waiting to be heard.
	answers := make(chan answer, len(vms))

	for _, uuid := range vms {
		go func() {
			inventory, err := r.daemons.Daemon(uuid).Inventory(ctx)
			answers <- answer{vm: uuid, inventory: inventory, err: err}
		}()
	}

	read := Read{At: at, Inventories: make(map[string]docker.Inventory, len(vms))}
	heard := make(map[string]bool, len(vms))

	for range vms {
		var a answer

		select {
		case a = <-answers:
		case <-ctx.Done():
		}

		if len(a.vm) == 0 {
			break
		}

		heard[a.vm] = true

		if a.err != nil {
			read.Unseen = append(read.Unseen, a.vm)

			continue
		}

		read.Inventories[a.vm] = a.inventory
	}

	// the VMs that did not answer before the read gave up on them.
	for _, uuid := range vms {
		if !heard[uuid] {
			read.Unseen = append(read.Unseen, uuid)
		}
	}

	slices.Sort(read.Unseen)

	return read, nil
}

// dockerVM is the uuid of a VM instance and whether it is a Docker VM: one
// its engine says is one, as its image does, or one a command named.
func (r *Reader) dockerVM(instance vm.Instance) (string, bool) {
	if instance.Labels[vm.LabelPurpose] != vm.PurposeVM {
		return "", false
	}

	uuid := instance.Labels[vm.LabelVM]
	if len(uuid) == 0 {
		uuid = instance.ID
	}

	if instance.Labels[vmKind.LabelDocker] == "true" {
		return uuid, true
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	return uuid, r.named[uuid]
}

// Reach is the dockerd of the Docker VM uuid names, once it answers: the VM
// has to be on this node, and running or coming up, and its dockerd is waited
// for as long as a Docker VM's is. One this node holds nothing of is
// vm.ErrNotRunning and ErrGone, and one that is neither running nor coming up
// is vm.ErrNotRunning.
func (r *Reader) Reach(ctx context.Context, vmUUID string) (docker.Daemon, error) {
	if len(vmUUID) == 0 {
		return nil, fmt.Errorf("%w: it names no vm", kind.ErrInvalidPayload)
	}

	instance, err := r.engine.Inspect(ctx, vmUUID)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		return nil, fmt.Errorf("%w: %w", vm.ErrNotRunning, ErrGone)
	case err != nil:
		return nil, err
	case instance.State != vm.InstanceRunning && instance.State != vm.InstanceCreated:
		return nil, fmt.Errorf("%w: its vm is %s", vm.ErrNotRunning, instance.State)
	}

	r.lock.Lock()
	r.named[vmUUID] = true
	r.lock.Unlock()

	daemon := r.daemons.Daemon(vmUUID)

	if err := daemon.Ping(ctx); err != nil {
		return nil, err
	}

	return daemon, nil
}

// Failure is what a command that failed with err says of it, in the codes
// every side knows.
func Failure(err error) *noderequest.Error {
	return noderequest.ErrorOf(err)
}

// Refused is err, a refusal dockerd answered a command with, as the command
// refused as it was asked (kind.ErrRefused) that it is when it is one: what
// it was asked of is as the command found it, and asked again, it would be
// refused again. Anything else is err as it is.
func Refused(err error) error {
	if errors.Is(err, docker.ErrInvalid) {
		return fmt.Errorf("%w: %w", kind.ErrRefused, err)
	}

	return err
}

// Cleared is what a command that did not fail says of the failure the one
// before it may have left: none.
func Cleared() *noderequest.Error {
	return &noderequest.Error{}
}

// Stacks are the stacks a Docker VM's containers say they belong to, by
// their compose projects: what a network or a volume compose made for a
// stack, which carries its project alone, belongs to.
func Stacks(inventory docker.Inventory) map[string]string {
	stacks := make(map[string]string)

	for _, c := range inventory.Containers {
		project, stack := c.Labels[docker.LabelComposeProject], c.Labels[stackKind.LabelStack]
		if len(project) > 0 && len(stack) > 0 {
			stacks[project] = stack
		}
	}

	return stacks
}

// Owners are what an object in a Docker VM belongs to: the VM, and the stack
// it is part of, which its labels name, or which the containers of its
// compose project name.
func Owners(vmUUID string, labels map[string]string, stacks map[string]string) []kind.Reference {
	owners := blockKinds.Owners(vmUUID, labels)

	if len(labels[stackKind.LabelStack]) > 0 {
		return owners
	}

	if stack := stacks[labels[docker.LabelComposeProject]]; len(stack) > 0 {
		owners = append(owners, kind.Reference{Kind: stackKind.Name, UUID: stack})
	}

	return owners
}
