// Package snapshot is the snapshot kind's node strategy: taking a snapshot
// of a VM this node holds, which is the one thing about a snapshot a node
// does.
//
// A snapshot is taken under the lock of the VM it is taken of, the one every
// command to that VM takes, so that the VM is not stopped, restored or
// deleted while its disk is read. The engine writes the VM's archive into a
// pipe and the bucket reads it out of the other end as one upload of unknown
// length, so it is streamed part by part and never held whole: a node has the
// memory for one part, not for a disk. A snapshot that fails leaves nothing
// behind in the bucket, and what came of it is the command's result either
// way, so the snapshot is never left being taken.
//
// A snapshot's create may reach a node more than once: delivered again, or
// sent again by the control plane while the first was still being carried
// out. What a node already took is answered with what that came to, rather
// than taken again over it, which would store a later disk under a record
// that says something else of it.
//
// Its state is its record's, in the control plane: a node holds nothing of a
// snapshot, and reports none.
package snapshot

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// remembered is how many snapshots taken here are remembered, to answer
	// a create of one of them that arrives again: far more than are taken
	// while one create could still be on its way.
	remembered = 256
)

// errStored is how the archive's reader is closed once the bucket has read
// all it is going to, which leaves the engine nothing to write to.
var errStored = errors.New("the archive is no longer being stored")

// Locks keeps what is done to one VM from overlapping: the locks every
// command to a VM takes.
type Locks interface {
	// Lock holds the lock of the VM vmUUID names, waiting for whoever holds
	// it now, and hands back what releases it. Giving up on waiting is ctx
	// ending, which holds nothing.
	Lock(ctx context.Context, vmUUID string) (release func(), err error)
}

// Recorder is where how long a snapshot took, and how large it came out, are
// recorded.
type Recorder interface {
	Snapshot(ctx context.Context, took time.Duration, size int64)
}

// Node is the snapshot kind's node strategy.
type Node struct {
	engine   vm.Engine
	archives snapshotKind.Store
	locks    Locks
	recorder Recorder
	now      func() time.Time

	lock sync.Mutex

	// taken is what came of each snapshot taken here, by uuid, and order the
	// uuids in the order they were taken, so the oldest is let go of first.
	taken map[string]taken
	order []string
}

// taken is what came of taking one snapshot.
type taken struct {
	status snapshotKind.Status
	err    error
}

var _ kind.Node[snapshotKind.Spec, snapshotKind.Status] = &Node{}

// New is the strategy that takes snapshots of the VMs engine runs, into
// archives, under the VMs' locks, recording each to recorder.
func New(engine vm.Engine, archives snapshotKind.Store, locks Locks, recorder Recorder) *Node {
	return &Node{
		engine:   engine,
		archives: archives,
		locks:    locks,
		recorder: recorder,
		now:      time.Now,
		taken:    make(map[string]taken),
	}
}

// Execute takes a snapshot of its VM's disk and stores it, and is the
// snapshot as that left it: ready, with how large its archive came out, the
// engine that wrote it and the disk a restore needs. A snapshot this node
// took already is what that came to.
func (n *Node) Execute(ctx context.Context, s snapshotKind.Snapshot, action string, _ any) (kind.Outcome[snapshotKind.Status], error) {
	if action != snapshotKind.ActionCreate {
		return kind.Outcome[snapshotKind.Status]{}, fmt.Errorf("%w: a snapshot cannot be %s on a node", kind.ErrUnknownAction, action)
	}

	vmUUID := snapshotKind.VMOf(s)
	if len(vmUUID) == 0 {
		return kind.Outcome[snapshotKind.Status]{}, fmt.Errorf("%w: the snapshot names no vm", kind.ErrInvalidPayload)
	}

	// one snapshot's commands are carried out one at a time, so one that
	// arrives again finds what the first came to here, without waiting for
	// its VM.
	if before, took := n.remembered(s.Metadata.UUID); took {
		return kind.Outcome[snapshotKind.Status]{Status: before.status}, before.err
	}

	release, err := n.locks.Lock(ctx, vmUUID)
	if err != nil {
		return kind.Outcome[snapshotKind.Status]{}, err
	}
	defer release()

	started := n.now()

	archive, size, err := n.take(ctx, s.Metadata.UUID, vmUUID)
	if err != nil {
		// a node going away has not failed the snapshot: whoever takes the
		// command next takes it.
		if ctx.Err() == nil {
			n.remember(s.Metadata.UUID, taken{err: err})
		}

		return kind.Outcome[snapshotKind.Status]{}, err
	}

	n.recorder.Snapshot(ctx, n.now().Sub(started), size)

	status := s.Status
	status.State = snapshotKind.Ready
	status.Reason = ""
	status.Engine = archive.Engine
	status.Size = size
	status.CompletedAt = n.now()

	if archive.Disk > 0 {
		status.Disk = archive.Disk
	}

	n.remember(s.Metadata.UUID, taken{status: status})

	return kind.Outcome[snapshotKind.Status]{Status: status}, nil
}

// take streams the VM's archive into the bucket, under the snapshot's object
// key, and says what the engine wrote and how much of it was stored.
func (n *Node) take(ctx context.Context, snapshotUUID string, vmUUID string) (vm.Archive, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, snapshotKind.TransferTimeout)
	defer cancel()

	key := snapshotKind.ObjectKey(snapshotUUID)

	reader, writer := io.Pipe()

	type written struct {
		archive vm.Archive
		err     error
	}

	wrote := make(chan written, 1)

	go func() {
		archive, err := n.engine.Snapshot(ctx, vmUUID, writer)

		// the end of the archive, or why it ends early: the bucket reads
		// either as the end of what it is storing.
		_ = writer.CloseWithError(err)
		wrote <- written{archive: archive, err: err}
	}()

	stored := &counter{reader: reader}
	storeErr := n.archives.Store(ctx, key, stored, -1)

	// a bucket that stopped reading lets the engine go rather than leave it
	// writing into a pipe nobody empties.
	_ = reader.CloseWithError(cmp.Or(storeErr, errStored))

	result := <-wrote

	if err := cmp.Or(result.err, storeErr); err != nil {
		// nothing of a snapshot that failed is left behind. An upload that
		// failed stored nothing anyway; this is for one that did not say so.
		_ = n.archives.Delete(context.WithoutCancel(ctx), key)

		return vm.Archive{}, 0, err
	}

	return result.archive, stored.read, nil
}

// remembered is what came of taking a snapshot here, if it was taken here.
func (n *Node) remembered(uuid string) (taken, bool) {
	n.lock.Lock()
	defer n.lock.Unlock()

	before, took := n.taken[uuid]

	return before, took
}

// remember keeps what came of taking a snapshot, letting go of the oldest
// once there are more than are remembered.
func (n *Node) remember(uuid string, outcome taken) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if _, kept := n.taken[uuid]; !kept {
		n.order = append(n.order, uuid)
	}

	n.taken[uuid] = outcome

	for len(n.order) > remembered {
		delete(n.taken, n.order[0])
		n.order = n.order[1:]
	}
}

// Query answers nothing: a snapshot has no query a node answers.
func (n *Node) Query(_ context.Context, _ snapshotKind.Snapshot, action string, _ any) (any, error) {
	return nil, fmt.Errorf("%w: a snapshot has no %q a node answers", kind.ErrUnknownAction, action)
}

// State is nothing: a snapshot's state is its record's, in the control
// plane, and a node holds nothing of one.
func (n *Node) State(context.Context) (kind.Report[snapshotKind.Status], error) {
	return kind.Report[snapshotKind.Status]{}, nil
}

// counter counts what is read through it, which is how much was stored.
type counter struct {
	reader io.Reader
	read   int64
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.read += int64(n)

	return n, err
}
